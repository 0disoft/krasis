package residentagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
	"github.com/0disoft/krasis/internal/wininput"
)

type productConnection struct {
	profile  truststore.ConnectionProfile
	peer     transport.AuthenticatedPeerContext
	tls      *tls.Config
	deadline time.Time
}

func prepareProductConnection(identities *productidentity.Store, trust *truststore.Store, alias string) (productConnection, error) {
	profile, err := trust.Connection(alias)
	if err != nil {
		return productConnection{}, err
	}
	document, err := trust.List()
	if err != nil {
		return productConnection{}, err
	}
	var peer transport.AuthenticatedPeerContext
	for _, record := range document.Peers {
		if record.Fingerprint == profile.Fingerprint && record.RevokedAt == nil {
			peer = transport.AuthenticatedPeerContext{CertificateSHA256: record.Fingerprint, Role: record.Role}
		}
	}
	leaf, err := trust.PeerCertificate(peer, time.Now())
	if err != nil {
		return productConnection{}, err
	}
	role := transport.AuthenticatedPeerListener
	if peer.Role == transport.AuthenticatedPeerListener {
		role = transport.AuthenticatedPeerConnector
	}
	identity, err := identities.OpenTransport(role, time.Now())
	if err != nil {
		return productConnection{}, err
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	var config *tls.Config
	if role == transport.AuthenticatedPeerListener {
		config, err = transport.NewServerTLSConfig(identity, roots)
	} else {
		config, err = transport.NewClientTLSConfig(identity, roots, productidentity.TransportServerName)
	}
	if err != nil {
		return productConnection{}, err
	}
	config.SessionTicketsDisabled = true
	config.VerifyConnection = func(state tls.ConnectionState) error {
		if len(state.PeerCertificates) != 1 {
			return errors.New("unexpected peer chain")
		}
		actual, err := transport.AuthenticatedPeerFromCertificate(state.PeerCertificates[0])
		if err != nil || actual != peer {
			return errors.New("wrong product peer")
		}
		_, err = trust.PeerCertificate(peer, time.Now())
		return err
	}
	deadline := leaf.NotAfter
	if identity.Leaf.NotAfter.Before(deadline) {
		deadline = identity.Leaf.NotAfter
	}
	return productConnection{profile: profile, peer: peer, tls: config, deadline: deadline}, nil
}

func productRunner(identities *productidentity.Store, trust *truststore.Store, log *productlog.Logger, auxiliary ...func(context.Context, string, productConnection)) SessionRunner {
	return func(parent context.Context, alias string, observer agent.SupervisorObserver) (err error) {
		prepared, err := prepareProductConnection(identities, trust, alias)
		if err != nil {
			return err
		}
		diagnostics, err := wininput.NewPlatform().Diagnose(parent)
		if err != nil {
			return err
		}
		if diagnostics.Display.MonitorCount != 1 {
			return errors.New("one active display is required")
		}
		deadlineCtx, cancel := context.WithDeadline(parent, prepared.deadline)
		defer cancel()
		ctx, cancelCause := context.WithCancelCause(deadlineCtx)
		defer cancelCause(context.Canceled)
		worker := installedProductInput(prepared.deadline, cancelCause)
		var input *wininput.InputAdapter
		if worker != nil {
			worker.event = log.Event
			defer worker.close(nil)
			input, err = wininput.NewInputAdapterWithInjection(prepared.profile.Rearm(), diagnostics.Display.Width, worker)
			log.Event("installed_input_selected")
		} else {
			input, err = wininput.NewInputAdapterWithReturnRearmDistance(prepared.profile.Rearm(), diagnostics.Display.Width)
		}
		if err != nil {
			return err
		}
		protectedActive := wininput.ProtectedInputActive
		if worker != nil {
			protectedActive = func() bool { return false }
		}
		defer func() {
			if errors.Is(context.Cause(ctx), crashguard.ErrUnexpectedExit) {
				err = errors.Join(err, crashguard.ErrUnexpectedExit)
			}
			if errors.Is(context.Cause(ctx), wininput.ErrDisplayGeometryChanged) {
				err = errors.Join(err, wininput.ErrDisplayGeometryChanged)
			}
			if errors.Is(context.Cause(ctx), wininput.ErrProtectedInput) {
				err = errors.Join(err, wininput.ErrProtectedInput)
			}
			if errors.Is(context.Cause(ctx), errPrivilegedInput) {
				err = errors.Join(err, errPrivilegedInput)
			}
		}()
		for _, start := range auxiliary {
			start(ctx, alias, prepared)
		}
		var source agent.ConnectionSource
		if prepared.peer.Role == transport.AuthenticatedPeerConnector {
			source, err = agent.NewListenSource(prepared.profile.Address, prepared.tls)
		} else {
			source = &boundedProductDial{
				load:        func() (truststore.ConnectionProfile, error) { return trust.Connection(alias) },
				fingerprint: prepared.peer.CertificateSHA256,
				tlsConfig:   prepared.tls,
			}
		}
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, source.Close()) }()
		if listener, ok := source.(*agent.ListenSource); ok {
			listener.SetAuthenticationFailureObserver(func(code string) { log.Event("preauth_" + code) })
		}
		source = &disconnectSource{ConnectionSource: source, intent: parent, log: log}
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		guard, err := crashguard.StartProduct(executable)
		if err != nil {
			return err
		}
		defer func() {
			closeErr := guard.Close()
			if closeErr != nil {
				log.Event("session_guard_close_failed")
			}
			err = errors.Join(err, closeErr)
		}()
		monitorDone := make(chan struct{})
		go func() {
			defer close(monitorDone)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			monitorProductSessionObserved(ctx, ticker.C, guard.Done(), func(trace *truststore.ReadTrace) error {
				_, checkErr := trust.PeerCertificateTraced(prepared.peer, time.Now(), trace)
				return checkErr
			}, cancel, func(code string) {
				log.Event(code)
				if code == "session_guard_exited" {
					cancelCause(crashguard.ErrUnexpectedExit)
				}
			}, func(snapshot truststore.ReadTraceSnapshot) {
				recordTrustTrace(snapshot, log.Event)
			})
		}()
		defer func() { cancel(); <-monitorDone }()
		var protectedOnce sync.Once
		stopProtected := func() {
			protectedOnce.Do(func() {
				log.Event("protected_input_detected")
				cancelCause(wininput.ErrProtectedInput)
			})
		}
		protectedDone := make(chan struct{})
		go func() {
			defer close(protectedDone)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			monitorProtectedInput(ctx, ticker.C, protectedActive, stopProtected)
		}()
		defer func() { cancel(); <-protectedDone }()
		config := agent.DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdge(prepared.profile.Edge), ScreenLeft: diagnostics.Display.Left, ScreenWidth: diagnostics.Display.Width, RearmDistance: prepared.profile.Rearm()})
		physicalWidth, physicalHeight, awareness := wininput.DisplayCoordinates()
		boundary := &boundaryDiagnostics{summary: productlog.BoundarySummary{
			Left: diagnostics.Display.Left, Top: diagnostics.Display.Top, Width: diagnostics.Display.Width, Height: diagnostics.Display.Height,
			PhysicalWidth: physicalWidth, PhysicalHeight: physicalHeight, DPIAwareness: awareness,
			Edge: string(config.Boundary.Edge), Gap: config.Boundary.ActivationGap, Rearm: config.Boundary.RearmDistance,
		}}
		log.Boundary(boundary.snapshot(false))
		defer func() { log.Boundary(boundary.snapshot(true)) }()
		config.SourceObserver = boundary
		progress := newRuntimeDiagnostics()
		config.LivenessObserver = progress
		config.QueueObserver = progress
		config.DestinationObserver = progress
		progressCtx, stopProgress := context.WithCancel(ctx)
		progressDone := make(chan struct{})
		go func() { defer close(progressDone); progress.run(progressCtx, log) }()
		defer func() { stopProgress(); <-progressDone }()
		config.ActivationFailureObserver = func(err error) {
			failure := agent.DescribeSessionFailure(err)
			log.Event(productlog.ActivationCode(failure.Component, failure.Phase, failure.Code))
		}
		// Initial conservative defaults, not a measured latency guarantee.
		config.Liveness = &agent.LivenessPolicy{HeartbeatInterval: 250 * time.Millisecond, PeerTimeout: 2 * time.Second}
		backoff := agent.DefaultBackoffPolicy()
		backoff.Jitter = true
		geometry := wininput.DisplayGeometry{Count: diagnostics.Display.MonitorCount, Left: diagnostics.Display.Left, Top: diagnostics.Display.Top, Width: diagnostics.Display.Width, Height: diagnostics.Display.Height}
		checkGeometry := displayGeometryGuard(ctx, geometry, wininput.CurrentDisplayGeometry, func() { log.Event("lifecycle_display_change"); cancelCause(wininput.ErrDisplayGeometryChanged) })
		hook := wininput.NewSourceHook(progress)
		hook.SetBoundaryGuard(checkGeometry)
		var inputGuard productInputGuard = guard
		if worker != nil {
			inputGuard = workerOwnedPressGuard{}
		}
		guarded := &guardedProductInput{InputBoundaryPort: input, guard: inputGuard, log: log, geometry: checkGeometry, protected: protectedActive, stopProtected: stopProtected}
		var boundaryInput agent.InputBoundaryPort = guarded
		var authorizer peer.LeaseAuthorizer = productAuthorizer{trust}
		runSession := agent.RunSession
		if worker != nil {
			boundaryInput = &privilegedBoundary{guardedProductInput: guarded, worker: worker}
			authorizer = privilegedAuthorizer{LeaseAuthorizer: authorizer, input: worker}
			runSession = func(ctx context.Context, config agent.Config, dependencies agent.Dependencies) (agent.Result, error) {
				defer worker.close(nil)
				return agent.RunSession(ctx, config, dependencies)
			}
		}
		return agent.RunSupervisor(ctx, agent.SupervisorConfig{Session: config, Backoff: backoff, Now: time.Now, Wait: agent.Wait, Observer: observer}, agent.SupervisorDependencies{
			Source: source, Hook: hook, Input: boundaryInput, Effects: agent.RuntimeEffects{}, Authorizer: authorizer, RunSession: runSession,
		})
	}
}

type boundedProductDial struct {
	load        func() (truststore.ConnectionProfile, error)
	fingerprint string
	tlsConfig   *tls.Config
}

func (source *boundedProductDial) Next(ctx context.Context) (agent.Connection, error) {
	handshake, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	profile, err := source.load()
	if err != nil {
		return nil, err
	}
	if profile.Fingerprint != source.fingerprint {
		return nil, errors.New("connection profile peer changed during reconnect")
	}
	dial, err := agent.NewDialSource(profile.Address, source.tlsConfig, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return dial.Next(handshake)
}

func (*boundedProductDial) Close() error { return nil }

type productAuthorizer struct{ trust *truststore.Store }

func (authorizer productAuthorizer) AuthorizeLease(ctx context.Context, peer transport.AuthenticatedPeerContext, session, epoch string, now time.Time) (string, bool, error) {
	if _, err := authorizer.trust.PeerCertificate(peer, now); err != nil {
		return "", false, err
	}
	document, err := authorizer.trust.List()
	if err != nil {
		return "", false, err
	}
	for _, record := range document.Peers {
		if record.Fingerprint == peer.CertificateSHA256 {
			delegate, err := agent.NewTrustedPeerAuthorizer([]agent.TrustedPeerGrant{{Peer: transport.AuthenticatedPeerContext{CertificateSHA256: record.Fingerprint, Role: record.Role}, AllowInboundControl: record.AllowInboundControl, Revoked: record.RevokedAt != nil}})
			if err != nil {
				return "", false, err
			}
			return delegate.AuthorizeLease(ctx, peer, session, epoch, now)
		}
	}
	return "", false, truststore.ErrPeerNotFound
}
