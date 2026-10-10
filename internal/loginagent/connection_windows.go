//go:build windows

package loginagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

// ConnectionDependencies are supplied by the authenticated service-child
// bootstrap, not CLI arguments or a peer. ReadGrant must read the protected
// machine file on every call. Hook/Input must own Winlogon desktop binding and
// pressed-input recovery. This runner neither creates nor grants those ports.
type ConnectionDependencies struct {
	ReadGrant func(time.Time) (Grant, bool, error)
	Hook      agent.SourceHook
	Input     agent.InputBoundaryPort
	Geometry  wininput.DisplayGeometry
}

type connectionPolicy struct {
	session     uint32
	profile     [32]byte
	grant       Grant
	live        Prepared
	readGrant   func(time.Time) (Grant, bool, error)
	readConsole func() (Console, error)
	readLive    func(Grant, time.Time) (Prepared, error)
}

func (p *connectionPolicy) check(now time.Time) (Prepared, error) {
	if p == nil || p.readGrant == nil || p.readConsole == nil || p.readLive == nil || p.session == 0 || p.session == ^uint32(0) || p.profile == ([32]byte{}) {
		return Prepared{}, ErrGrant
	}
	console, err := p.readConsole()
	if err != nil || console != (Console{Session: p.session, State: BeforeLogin}) {
		return Prepared{}, ErrObservation
	}
	g, present, err := p.readGrant(now)
	if err != nil || !present {
		return Prepared{}, ErrGrant
	}
	selection, err := g.Selection(now)
	if err != nil || selection.Profile != p.profile || g != p.grant {
		return Prepared{}, ErrGrant
	}
	live, err := p.readLive(g, now)
	if err != nil {
		return Prepared{}, err
	}
	// A changed address/layout or inbound permission ends this connection too.
	// Never reconnect or keep an existing lease under a new user policy.
	if live.Profile != p.live.Profile || live.Peer.AllowInboundControl != p.live.Peer.AllowInboundControl || live.ExpiresAt != p.live.ExpiresAt {
		return Prepared{}, ErrGrant
	}
	console, err = p.readConsole()
	if err != nil || console != (Console{Session: p.session, State: BeforeLogin}) {
		return Prepared{}, ErrObservation
	}
	return live, nil
}

func (p *connectionPolicy) tlsConfig() (*tls.Config, error) {
	if _, err := p.check(time.Now().UTC()); err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	roots.AddCert(p.live.Remote)
	var config *tls.Config
	var err error
	if p.grant.Role == transport.AuthenticatedPeerConnector {
		config, err = transport.NewServerTLSConfig(p.live.Local, roots)
	} else {
		config, err = transport.NewClientTLSConfig(p.live.Local, roots, productidentity.TransportServerName)
	}
	if err != nil {
		return nil, err
	}
	config.SessionTicketsDisabled = true
	config.VerifyConnection = func(state tls.ConnectionState) error {
		if state.Version != tls.VersionTLS13 || state.NegotiatedProtocol != transport.ALPNProtocol || len(state.PeerCertificates) != 1 || len(state.VerifiedChains) == 0 || state.DidResume {
			return ErrGrant
		}
		actual, err := transport.AuthenticatedPeerFromCertificate(state.PeerCertificates[0])
		if err != nil || actual != (transport.AuthenticatedPeerContext{CertificateSHA256: p.grant.Peer, Role: p.grant.Role}) {
			return ErrGrant
		}
		_, err = p.check(time.Now().UTC())
		return err
	}
	return config, nil
}

func (p *connectionPolicy) AuthorizeLease(ctx context.Context, remote transport.AuthenticatedPeerContext, session, epoch string, now time.Time) (string, bool, error) {
	if ctx == nil || ctx.Err() != nil || remote != (transport.AuthenticatedPeerContext{CertificateSHA256: p.grant.Peer, Role: p.grant.Role}) {
		return "", false, ErrGrant
	}
	live, err := p.check(now)
	if err != nil {
		return "", false, err
	}
	delegate, err := agent.NewTrustedPeerAuthorizer([]agent.TrustedPeerGrant{{Peer: remote, AllowInboundControl: live.Peer.AllowInboundControl}})
	if err != nil {
		return "", false, err
	}
	return delegate.AuthorizeLease(ctx, remote, session, epoch, now)
}

// RunConnection exposes only the existing input session protocol for one pinned
// peer. No management registry, trust mutation, clipboard, update, discovery,
// credential rotation, user API or diagnostic content collection is installed.
// It remains unwired until service-child authentication and desktop cleanup
// acknowledgement are implemented; supplying ports is not process authority.
func RunConnection(ctx context.Context, session uint32, profile [32]byte, deps ConnectionDependencies) (result error) {
	if ctx == nil || ctx.Err() != nil || session == 0 || session == ^uint32(0) || profile == ([32]byte{}) || deps.ReadGrant == nil || deps.Hook == nil || deps.Input == nil || !deps.Geometry.Valid() {
		return ErrInvalid
	}
	now := time.Now().UTC()
	g, present, err := deps.ReadGrant(now)
	if err != nil || !present {
		return ErrGrant
	}
	live, err := ReadGrantConnection(g, now)
	if err != nil {
		return err
	}
	if live.Profile.Rearm() >= deps.Geometry.Width {
		return ErrInvalid
	}
	p := &connectionPolicy{session: session, profile: profile, grant: g, live: live, readGrant: deps.ReadGrant, readConsole: ReadConsole, readLive: ReadGrantConnection}
	tlsConfig, err := p.tlsConfig()
	if err != nil {
		return err
	}
	deadline, cancelDeadline := context.WithDeadline(ctx, live.ExpiresAt)
	defer cancelDeadline()
	owned, cancel := context.WithCancelCause(deadline)
	defer cancel(context.Canceled)
	var source agent.ConnectionSource
	if g.Role == transport.AuthenticatedPeerConnector {
		source, err = agent.NewListenSource(live.Profile.Address, tlsConfig)
	} else {
		source, err = agent.NewDialSource(live.Profile.Address, tlsConfig, 2*time.Second)
	}
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, source.Close()) }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		monitorConnectionPolicy(owned, ticker.C, func(now time.Time) (Prepared, error) {
			if wininput.CurrentDisplayGeometry() != deps.Geometry {
				return Prepared{}, wininput.ErrDisplayGeometryChanged
			}
			return p.check(now)
		}, cancel)
	}()
	defer func() { cancel(context.Canceled); <-done }()
	config := agent.DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdge(live.Profile.Edge), RearmDistance: live.Profile.Rearm(), ScreenLeft: deps.Geometry.Left, ScreenWidth: deps.Geometry.Width})
	config.Liveness = &agent.LivenessPolicy{HeartbeatInterval: 250 * time.Millisecond, PeerTimeout: 2 * time.Second}
	backoff := agent.DefaultBackoffPolicy()
	backoff.Jitter = true
	result = agent.RunSupervisor(owned, agent.SupervisorConfig{Session: config, Backoff: backoff, Now: time.Now, Wait: agent.Wait}, agent.SupervisorDependencies{
		Source: &checkedConnectionSource{source: source, check: p.check}, Hook: deps.Hook, Input: deps.Input,
		Effects: agent.RuntimeEffects{}, Authorizer: p, RunSession: agent.RunSession,
	})
	if cause := context.Cause(owned); cause != nil && !errors.Is(cause, context.Canceled) {
		result = errors.Join(result, cause)
	}
	return result
}

func monitorConnectionPolicy(ctx context.Context, ticks <-chan time.Time, check func(time.Time) (Prepared, error), cancel context.CancelCauseFunc) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok {
				cancel(ErrObservation)
				return
			}
			if _, err := check(time.Now().UTC()); err != nil {
				cancel(err)
				return
			}
		}
	}
}

type checkedConnectionSource struct {
	source agent.ConnectionSource
	check  func(time.Time) (Prepared, error)
}

func (s *checkedConnectionSource) Next(ctx context.Context) (agent.Connection, error) {
	if _, err := s.check(time.Now().UTC()); err != nil {
		return nil, err
	}
	connection, err := s.source.Next(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.check(time.Now().UTC()); err != nil {
		return nil, errors.Join(err, connection.Close())
	}
	return connection, nil
}

func (s *checkedConnectionSource) Close() error { return s.source.Close() }
