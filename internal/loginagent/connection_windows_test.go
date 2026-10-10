//go:build windows

package loginagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/transport"
)

func connectionPolicyFixture(t *testing.T) (*connectionPolicy, string) {
	t.Helper()
	g, base, _, _ := liveGrantFixture(t)
	now := time.Now().UTC()
	selection, err := g.Selection(now)
	if err != nil {
		t.Fatal(err)
	}
	live, err := readGrantConnectionAt(g, base, now)
	if err != nil {
		t.Fatal(err)
	}
	return &connectionPolicy{
		session: 1, profile: selection.Profile, grant: g, live: live,
		readGrant:   func(time.Time) (Grant, bool, error) { return g, true, nil },
		readConsole: func() (Console, error) { return Console{Session: 1, State: BeforeLogin}, nil },
		readLive:    func(g Grant, now time.Time) (Prepared, error) { return readGrantConnectionAt(g, base, now) },
	}, base
}

func TestLoginTLSIsMutualPinnedAndRechecksPolicy(t *testing.T) {
	p, base := connectionPolicyFixture(t)
	config, err := p.tlsConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.InsecureSkipVerify || !config.SessionTicketsDisabled || config.MinVersion != tls.VersionTLS13 || config.MaxVersion != tls.VersionTLS13 || len(config.Certificates) != 1 || len(config.NextProtos) != 1 || config.NextProtos[0] != transport.ALPNProtocol {
		t.Fatal("login transport loosened ordinary mTLS")
	}
	state := tls.ConnectionState{Version: tls.VersionTLS13, NegotiatedProtocol: transport.ALPNProtocol, PeerCertificates: []*x509.Certificate{p.live.Remote}, VerifiedChains: [][]*x509.Certificate{{p.live.Remote}}}
	if err := config.VerifyConnection(state); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*tls.ConnectionState){
		func(s *tls.ConnectionState) { s.Version = tls.VersionTLS12 },
		func(s *tls.ConnectionState) { s.NegotiatedProtocol = transport.ALPNInputPreparation },
		func(s *tls.ConnectionState) { s.DidResume = true },
		func(s *tls.ConnectionState) { s.VerifiedChains = nil },
		func(s *tls.ConnectionState) { s.PeerCertificates = []*x509.Certificate{p.live.Local.Leaf} },
		func(s *tls.ConnectionState) { s.PeerCertificates = append(s.PeerCertificates, p.live.Remote) },
	} {
		other := state
		change(&other)
		if err := config.VerifyConnection(other); err == nil {
			t.Fatal("unexpected TLS peer/protocol accepted")
		}
	}
	if err := appsettings.SaveResume(base, appsettings.Resume{ManualStop: true}); err != nil {
		t.Fatal(err)
	}
	if err := config.VerifyConnection(state); !errors.Is(err, ErrSuspended) {
		t.Fatal("handshake ignored fresh manual disconnect", err)
	}
}

func TestLoginTransportPerformsRealMutualTLSWithoutNativeInput(t *testing.T) {
	g, base, _, _, remote := liveGrantFixtureWithRemote(t)
	now := time.Now().UTC()
	selection, err := g.Selection(now)
	if err != nil {
		t.Fatal(err)
	}
	live, err := readGrantConnectionAt(g, base, now)
	if err != nil {
		t.Fatal(err)
	}
	p := &connectionPolicy{session: 1, profile: selection.Profile, grant: g, live: live,
		readGrant:   func(time.Time) (Grant, bool, error) { return g, true, nil },
		readConsole: func() (Console, error) { return Console{Session: 1, State: BeforeLogin}, nil },
		readLive:    func(g Grant, now time.Time) (Prepared, error) { return readGrantConnectionAt(g, base, now) },
	}
	clientConfig, err := p.tlsConfig()
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(live.Local.Leaf)
	serverConfig, err := transport.NewServerTLSConfig(remote, roots)
	if err != nil {
		t.Fatal(err)
	}
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	server, client := tls.Server(left, serverConfig), tls.Client(right, clientConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 2)
	go func() { done <- server.HandshakeContext(ctx) }()
	go func() { done <- client.HandshakeContext(ctx) }()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal("mutual handshake failed", err)
		}
	}
	actual, err := transport.AuthenticatedPeerFromCertificate(client.ConnectionState().PeerCertificates[0])
	if err != nil || actual.CertificateSHA256 != g.Peer || actual.Role != g.Role {
		t.Fatal("handshake selected wrong peer", err)
	}
}

func TestLoginPolicyRejectsRemovalPendingChangesAndLogon(t *testing.T) {
	p, _ := connectionPolicyFixture(t)
	if _, err := p.check(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, read := range []func(time.Time) (Grant, bool, error){
		func(time.Time) (Grant, bool, error) { return Grant{}, false, nil },
		func(time.Time) (Grant, bool, error) { return p.grant, true, errors.New("pending") },
		func(time.Time) (Grant, bool, error) {
			other := p.grant
			other.ID = strings.Repeat("b", 64)
			return other, true, nil
		},
	} {
		other := *p
		other.readGrant = read
		if _, err := other.check(time.Now().UTC()); !errors.Is(err, ErrGrant) {
			t.Fatal("machine grant change retained connection", err)
		}
	}
	for _, console := range []Console{{Session: 1, State: UserSession}, {Session: 2, State: BeforeLogin}, {State: Unavailable}, {State: Unknown}} {
		other := *p
		other.readConsole = func() (Console, error) { return console, nil }
		if _, err := other.check(time.Now().UTC()); !errors.Is(err, ErrObservation) {
			t.Fatal("logon or switched console retained connection", err)
		}
	}
	for _, change := range []func(*Prepared){
		func(live *Prepared) { live.Profile.Address = "127.0.0.1:24801" },
		func(live *Prepared) { live.Profile.Edge = "LEFT" },
		func(live *Prepared) { live.Peer.AllowInboundControl = false },
		func(live *Prepared) { live.ExpiresAt = live.ExpiresAt.Add(time.Second) },
	} {
		other := *p
		other.readLive = func(Grant, time.Time) (Prepared, error) { live := p.live; change(&live); return live, nil }
		if _, err := other.check(time.Now().UTC()); !errors.Is(err, ErrGrant) {
			t.Fatal("profile change retained old lease", err)
		}
	}
}

func TestLoginLeaseRechecksGrantAndInboundCap(t *testing.T) {
	p, _ := connectionPolicyFixture(t)
	remote := transport.AuthenticatedPeerContext{CertificateSHA256: p.grant.Peer, Role: p.grant.Role}
	if lease, allowed, err := p.AuthorizeLease(context.Background(), remote, "session", "epoch", time.Now().UTC()); err != nil || !allowed || lease == "" {
		t.Fatal("authorized input refused", err)
	}
	wrong := remote
	wrong.CertificateSHA256 = strings.Repeat("c", 64)
	if _, allowed, err := p.AuthorizeLease(context.Background(), wrong, "session", "epoch", time.Now().UTC()); allowed || !errors.Is(err, ErrGrant) {
		t.Fatal("wrong peer authorized")
	}
	p.live.Peer.AllowInboundControl = false
	p.readLive = func(Grant, time.Time) (Prepared, error) { return p.live, nil }
	if lease, allowed, err := p.AuthorizeLease(context.Background(), remote, "session", "epoch", time.Now().UTC()); err != nil || allowed || lease != "" {
		t.Fatal("denied inbound permission escalated", err)
	}
	p.readGrant = func(time.Time) (Grant, bool, error) { return Grant{}, false, nil }
	if _, allowed, err := p.AuthorizeLease(context.Background(), remote, "session", "epoch", time.Now().UTC()); allowed || !errors.Is(err, ErrGrant) {
		t.Fatal("removed grant still authorized input")
	}
}

func TestLoginPolicyMonitorPreservesRevocationCauseAndJoins(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	ticks := make(chan time.Time, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		monitorConnectionPolicy(ctx, ticks, func(time.Time) (Prepared, error) { return Prepared{}, ErrSuspended }, cancel)
	}()
	ticks <- time.Now()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("policy monitor did not stop")
	}
	if !errors.Is(context.Cause(ctx), ErrSuspended) {
		t.Fatal("revocation cause lost")
	}
	closedCtx, closedCancel := context.WithCancelCause(context.Background())
	defer closedCancel(context.Canceled)
	close(ticks)
	monitorConnectionPolicy(closedCtx, ticks, func(time.Time) (Prepared, error) { t.Fatal("closed channel retried policy"); return Prepared{}, nil }, closedCancel)
	if !errors.Is(context.Cause(closedCtx), ErrObservation) {
		t.Fatal("observation termination ignored")
	}
}
