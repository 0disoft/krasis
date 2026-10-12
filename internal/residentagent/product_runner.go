package residentagent

import (
	"bytes"
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
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/peerdiscovery"
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
	anchor   *x509.Certificate
}

func prepareProductConnection(identities *productidentity.Store, trust *truststore.Store, alias string, selected ...string) (productConnection, error) {
	profile, err := trust.Connection(alias)
	if err != nil {
		return productConnection{}, err
	}
	if len(selected) > 0 && selected[0] != "" && profile.Fingerprint != selected[0] {
		return productConnection{}, errors.New("selected peer changed")
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
	leaf, err := trust.PeerKeyAnchor(peer, time.Now())
	if err != nil {
		return productConnection{}, err
	}
	role := transport.AuthenticatedPeerListener
	if peer.Role == transport.AuthenticatedPeerListener {
		role = transport.AuthenticatedPeerConnector
	}
	identity, err := identities.OpenTransportAnchor(role)
	if errors.Is(err, os.ErrNotExist) {
		identity, err = identities.OpenTransport(role, time.Now())
	}
	if err != nil {
		return productConnection{}, err
	}
	localKey, err := transport.ProductKeyFingerprint(identity.Leaf)
	if err != nil {
		return productConnection{}, err
	}
	local := func() (tls.Certificate, error) {
		current, err := identities.OpenTransportAnchor(role)
		if err != nil {
			return tls.Certificate{}, err
		}
		key, err := transport.ProductKeyFingerprint(current.Leaf)
		if err != nil || key != localKey {
			return tls.Certificate{}, errors.New("local paired key changed")
		}
		return productidentity.RenewableTransport(current, time.Now())
	}
	config, err := transport.NewProductKeyTLSConfig(local, leaf, role == transport.AuthenticatedPeerListener, func() error {
		current, err := trust.PeerKeyAnchor(peer, time.Now())
		if err != nil {
			return err
		}
		if !bytes.Equal(current.Raw, leaf.Raw) {
			return errors.New("paired key anchor changed")
		}
		return nil
	})
	if err != nil {
		return productConnection{}, err
	}
	transport.EnableInputPreparation(config)
	return productConnection{profile: profile, peer: peer, tls: config, anchor: leaf}, nil
}

func productRunner(identities *productidentity.Store, trust *truststore.Store, log *productlog.Logger, auxiliary ...func(context.Context, string, productConnection)) SessionRunner {
	profiles := handoff.New(log.Handoff)
	return func(parent context.Context, alias string, observer agent.SupervisorObserver) (err error) {
		prepared, err := prepareProductConnection(identities, trust, alias, localapi.PeerFingerprint(parent))
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
		deadlineCtx, cancel := productConnectionContext(parent, prepared.deadline)
		defer cancel()
		ctx, cancelCause := context.WithCancelCause(deadlineCtx)
		defer cancelCause(context.Canceled)
		worker := installedProductInput(prepared.deadline, cancelCause)
		var input *wininput.InputAdapter
		if worker != nil {
			worker.event = log.Event
			defer func() { err = errors.Join(err, worker.shutdown()) }()
			input, err = wininput.NewInputAdapterWithInjection(prepared.profile.Rearm(), diagnostics.Display.Width, worker)
			log.Event("installed_input_selected")
		} else {
			input, err = wininput.NewInputAdapterWithReturnRearmDistance(prepared.profile.Rearm(), diagnostics.Display.Width)
		}
		if err != nil {
			return err
		}
		protectedActive := func() bool { return false }
		if reporter, ok := observer.(interface{ ObserveInputBackend(bool) }); ok {
			reporter.ObserveInputBackend(worker != nil)
		}
		if worker == nil {
			protectedActive = observedProtectedInput(wininput.InspectProtectedInput, log.Event)
		}
		defer func() {
			if failure := agent.DescribeSessionFailure(context.Cause(ctx)); failure.Component == "session-trust" {
				err = errors.Join(err, context.Cause(ctx))
			}
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
			source, err = newProductListen(ctx, prepared.profile, prepared.tls, func() (truststore.ConnectionProfile, error) { return trust.Connection(alias) }, func(p truststore.ConnectionProfile, address string) error {
				_, err := trust.SetConnectionAddress(alias, p.Fingerprint, p.Address, address)
				return err
			}, log.Event, prepared.anchor)
		} else {
			source = &boundedProductDial{
				load:        func() (truststore.ConnectionProfile, error) { return trust.Connection(alias) },
				fingerprint: prepared.peer.CertificateSHA256,
				tlsConfig:   prepared.tls,
				anchor:      prepared.anchor,
				discover:    peerdiscovery.Discover,
				save: func(p truststore.ConnectionProfile, address string) error {
					_, err := trust.SetConnectionAddress(alias, p.Fingerprint, p.Address, address)
					return err
				},
				observe: log.Event,
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
				_, checkErr := trust.PeerKeyAnchorTraced(prepared.peer, time.Now(), trace)
				return checkErr
			}, cancel, func(code string) {
				log.Event(code)
				if cause := sessionTrustFailure(code); cause != nil {
					cancelCause(cause)
				}
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
		if flow, ok := observer.(peer.ControlFlowObserver); ok {
			config.ControlObserver = flow
		}
		config.HandoffProfiles = profiles
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
		hook.SetPresentationReporter(log.Presentation)
		hook.SetAdmissionReporter(log.Event)
		hook.SetBoundaryGuard(checkGeometry)
		var sessionHook agent.SourceHook = hook
		if worker != nil {
			sessionHook = protectedSessionHook(hook, log.Event)
		}
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
			Source: source, Hook: sessionHook, Input: boundaryInput, Effects: agent.RuntimeEffects{}, Authorizer: authorizer, RunSession: runSession,
		})
	}
}

type productAuthorizer struct{ trust *truststore.Store }

func (authorizer productAuthorizer) AuthorizeLease(ctx context.Context, peer transport.AuthenticatedPeerContext, session, epoch string, now time.Time) (string, bool, error) {
	if _, err := authorizer.trust.PeerKeyAnchor(peer, now); err != nil {
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
