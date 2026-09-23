package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

type SourceHook interface {
	RunWithBoundary(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error
}

type sourceCaptureProvider interface {
	SourceCapture() wininput.SourceActivation
}

type InputBoundaryPort interface {
	application.InputPort
	peer.CursorEntryPort
	peer.ReturnBoundaryPort
	ReturnIntents() <-chan control.ReturnIntent
}

type Connection interface {
	peer.FrameWriter
	peer.SessionConnection
	AuthenticatedPeer() (transport.AuthenticatedPeerContext, error)
}

type Config struct {
	Boundary                  control.BoundaryConfig
	QueueCapacity             int
	Now                       peer.ObservationClock
	Liveness                  *LivenessPolicy
	LivenessObserver          peer.LivenessObserver
	SourceObserver            SourceActivityObserver
	PipelineObserver          wininput.SourcePipelineObserver
	QueueObserver             wininput.SourceQueueObserver
	ControlObserver           peer.ControlFlowObserver
	DestinationObserver       application.InputDispositionObserver
	RecoveryObserver          func(RecoveryReport)
	ActivationFailureObserver func(error)
}

type SourceActivityObserver interface {
	ObserveSourceAction(control.InputAction)
	ObserveBoundaryIntent(control.BoundaryIntent)
}

type Dependencies struct {
	Connection   Connection
	Hook         SourceHook
	Input        InputBoundaryPort
	Effects      application.EffectExecutor
	Authorizer   peer.LeaseAuthorizer
	Management   *SessionRegistry
	TrustRevoker TrustRevoker
}

type Result struct {
	FinalState control.State
	Cause      error
	Recovery   RecoveryReport
}

func RunSession(ctx context.Context, config Config, dependencies Dependencies) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("agent context is required")
	}
	if err := validate(config, dependencies); err != nil {
		return Result{}, err
	}
	connection := newSessionConnectionEvidence(dependencies.Connection, config.ControlObserver)
	authenticatedPeer, err := connection.AuthenticatedPeer()
	if err != nil {
		return Result{}, err
	}
	detector, err := control.NewBoundaryDetector(config.Boundary)
	if err != nil {
		return Result{}, err
	}
	var observeBoundary func(control.BoundaryIntent)
	if config.SourceObserver != nil {
		observeBoundary = config.SourceObserver.ObserveBoundaryIntent
	}
	emitter, err := control.NewBoundaryIntentEmitterObserved(detector, observeBoundary)
	if err != nil {
		return Result{}, err
	}
	runtime, err := application.NewSessionRuntimeObserved(
		dependencies.Effects,
		dependencies.Input,
		config.DestinationObserver,
	)
	if err != nil {
		return Result{}, err
	}
	coordinator, err := peer.NewSessionWriteCoordinator(connection)
	if err != nil {
		return Result{}, err
	}
	actionSink, err := peer.NewOutboundActionSink(coordinator, runtime, connection)
	if err != nil {
		return Result{}, err
	}
	var sourceSink wininput.SourceSink = actionSink
	if config.SourceObserver != nil {
		sourceSink = &observedSourceSink{delegate: actionSink, observer: config.SourceObserver}
	}
	source, err := wininput.NewSourceSessionWithObservers(config.QueueCapacity, sourceSink, config.PipelineObserver, config.QueueObserver)
	if err != nil {
		return Result{}, err
	}
	emergency, err := wininput.NewEmergencyBoundaryObserver(source, emitter)
	if err != nil {
		return Result{}, err
	}
	var outboundSource peer.SourceActivation = source
	if provider, ok := dependencies.Hook.(sourceCaptureProvider); ok {
		capture := provider.SourceCapture()
		if capture == nil {
			return Result{}, errors.New("source capture provider returned no activation owner")
		}
		outboundSource = &combinedSourceActivation{source: source, capture: capture, observer: config.ControlObserver, failureObserver: config.ActivationFailureObserver}
	}
	outbound, err := peer.NewOutboundControllerObserved(coordinator, runtime, outboundSource, config.ControlObserver)
	if err != nil {
		return Result{}, err
	}
	var admissionGate peer.BoundaryAdmissionGate
	if gate, ok := dependencies.Hook.(peer.BoundaryAdmissionGate); ok {
		admissionGate = gate
	}
	returns, err := peer.NewReturnControllerObserved(coordinator, runtime, dependencies.Input, config.ControlObserver)
	if err != nil {
		return Result{}, err
	}
	orchestrator, err := peer.NewBoundaryOrchestratorObservedWithGate(
		&peer.BidirectionalBoundary{Outbound: outbound, Return: returns},
		peer.NewRandomOutboundIdentity,
		config.ControlObserver,
		admissionGate,
	)
	if err != nil {
		return Result{}, err
	}
	responder := &peer.LeaseResponder{
		Coordinator: coordinator, Runtime: runtime, Authorizer: dependencies.Authorizer,
		Peer: authenticatedPeer, Entry: dependencies.Input, Returns: dependencies.Input, Observer: config.ControlObserver,
	}
	if cursor, ok := dependencies.Hook.(peer.ReturnCursorPresentation); ok {
		returns.Cursor = cursor
		responder.Cursor = cursor
		defer cursor.RestoreReturnCursor()
	}
	if dependencies.Management != nil {
		managed, managedErr := newManagedPeerSession(
			authenticatedPeer,
			runtime,
			coordinator,
			outboundSource,
			connection,
			dependencies.TrustRevoker,
			config.Now,
		)
		if managedErr != nil {
			return Result{}, managedErr
		}
		registration, registerErr := dependencies.Management.register(managed)
		if registerErr != nil {
			return Result{}, registerErr
		}
		defer registration.Close()
	}

	runContext, cancel := context.WithCancelCause(ctx)
	defer cancel(context.Canceled)
	var peerTimeout time.Duration
	if config.Liveness != nil {
		peerTimeout = config.Liveness.PeerTimeout
	}
	sourceReady := make(chan struct{})
	hookReady := make(chan struct{})
	components := []component{
		{name: "source-session", run: func() error {
			return source.RunWithReady(runContext, func() { close(sourceReady) })
		}},
		{name: "source-hook", wait: sourceReady, run: func() error {
			return dependencies.Hook.RunWithBoundary(runContext, source, emergency, func() { close(hookReady) })
		}},
		{name: "boundary-orchestrator", wait: hookReady, run: func() error {
			return orchestrator.Run(runContext, emitter.Intents())
		}},
		{name: "return-controller", run: func() error {
			return returns.Run(runContext, dependencies.Input.ReturnIntents())
		}},
		{name: "peer-session", run: func() error {
			return peer.RunSessionWithLifecycleObserved(runContext, connection, coordinator, runtime, responder, outbound, config.Now, peerTimeout, config.LivenessObserver)
		}},
	}
	if config.Liveness != nil {
		policy := *config.Liveness
		components = append(components, component{name: "lease-watchdog", run: func() error {
			return runLeaseWatchdog(runContext, runtime.LeaseWatch, policy, func(cause error) {
				source.Retire()
				cancel(cause)
				_ = connection.Close()
			})
		}})
		components = append(components, component{name: "peer-liveness", run: func() error {
			return runLiveness(runContext, coordinator, runtime, policy, config.Now, config.LivenessObserver)
		}})
	}

	var workers sync.WaitGroup
	outcomes := make([]error, len(components))
	workers.Add(len(components))
	for index, current := range components {
		current := current
		go func() {
			defer workers.Done()
			if current.wait != nil {
				select {
				case <-current.wait:
				case <-runContext.Done():
					return
				}
			}
			runErr := current.run()
			if runErr == nil && runContext.Err() == nil {
				runErr = fmt.Errorf("%s stopped unexpectedly", current.name)
			}
			if runErr != nil {
				outcomes[index] = &sessionComponentError{component: current.name, cause: runErr}
				if runContext.Err() == nil {
					cancel(outcomes[index])
				}
			}
		}()
	}
	<-runContext.Done()
	source.Retire()
	joined := make(chan struct{})
	go func() { workers.Wait(); close(joined) }()
	expired := awaitRecovery(joined, 2*wininput.SourceCleanupTimeout, config.RecoveryObserver)
	cause := context.Cause(runContext)
	if expired {
		cause = errors.Join(cause, ErrRecoveryUnverified)
	}
	foregroundOwnerReleased := false
	// Wait establishes that every slot is final. Cancellation selects the first
	// cause; it must not discard errors produced later by native/input cleanup.
	for index, outcome := range outcomes {
		if components[index].name == "lease-watchdog" && errors.Is(outcome, cause) {
			continue
		}
		if outcome != nil && !expectedWorkerCancellation(outcome, runContext.Err()) {
			if components[index].name == "source-session" &&
				errors.Is(cause, wininput.ErrSourceCaptureLostForeground) &&
				foregroundLossCleanupOnly(outcome, false) {
				// The source worker can report the same verified foreground
				// loss after another component wins cancellation.
				cause = errors.Join(cause, outcome)
				continue
			}
			if components[index].name == "source-hook" &&
				errors.Is(cause, wininput.ErrSourceCaptureLostForeground) &&
				foregroundLossCleanupOnly(outcome, false) {
				// The capture owner exited after foreground loss. Its joined
				// cancellation is expected; keep the cause for reconnect telemetry.
				cause = errors.Join(cause, outcome)
				foregroundOwnerReleased = true
				continue
			}
			if components[index].name == "peer-session" &&
				errors.Is(cause, wininput.ErrSourceCaptureLostForeground) &&
				foregroundLossCleanupOnly(outcome, true) {
				// The read ended because this source closed its own connection.
				// Keep the diagnostic without treating that closure as a second fault.
				cause = errors.Join(cause, outcome)
				continue
			}
			if !errors.Is(cause, outcome) {
				cause = errors.Join(cause, outcome, ErrRecoveryUnverified)
			} else if components[index].name == "source-hook" {
				cause = errors.Join(cause, ErrRecoveryUnverified)
			}
		}
	}
	recovery := RecoveryReport{ForwardingStopped: RecoveryConfirmed, WorkersJoined: RecoveryConfirmed, DeadlineExceeded: expired}
	// A timed-out cleanup helper may still own the runtime mutex. Do not try
	// to turn an already-unverified recovery into another unbounded snapshot.
	if errors.Is(cause, ErrRecoveryUnverified) {
		return Result{FinalState: runtime.LeaseWatch().State, Cause: cause, Recovery: recovery}, cause
	}
	snapshot := runtime.Snapshot()
	for index, current := range components {
		if current.name == "source-hook" && (foregroundOwnerReleased || expectedWorkerCancellation(outcomes[index], runContext.Err())) {
			recovery.NativeOwnersReleased = RecoveryConfirmed
		}
	}
	if snapshot.PressedCount == 0 {
		recovery.PressedLedgerEmpty = RecoveryConfirmed
	}
	result := Result{FinalState: snapshot.Control.State, Cause: cause, Recovery: recovery}
	if snapshot.Control.State != control.StateLocal {
		return result, errors.Join(fmt.Errorf("agent session stopped outside LOCAL: %s", snapshot), cause, ErrRecoveryUnverified)
	}
	if snapshot.PressedCount != 0 {
		return result, errors.Join(cause, ErrRecoveryUnverified)
	}
	return result, cause
}

// A foreground-loss owner exit may contain only its cause and cancellation.
// Its peer read may also contain the local interruption and a closed stream.
// Any unrelated cleanup failure keeps recovery unverified.
func foregroundLossCleanupOnly(err error, peerRead bool) bool {
	seenLoss := false
	seenInterrupt := false
	seenClosed := false
	var check func(error) bool
	check = func(current error) bool {
		if current == nil {
			return true
		}
		if joined, ok := current.(interface{ Unwrap() []error }); ok {
			for _, child := range joined.Unwrap() {
				if !check(child) {
					return false
				}
			}
			return true
		}
		if wrapped, ok := current.(interface{ Unwrap() error }); ok {
			return check(wrapped.Unwrap())
		}
		if current == wininput.ErrSourceCaptureLostForeground {
			seenLoss = true
			return true
		}
		if current == context.Canceled {
			return true
		}
		if !peerRead {
			return false
		}
		switch current {
		case peer.ErrPeerReadAfterLocalInterrupt:
			seenInterrupt = true
			return true
		case peer.ErrSourceSessionInterrupted, transport.ErrInvalidFrame:
			return true
		case io.ErrClosedPipe, net.ErrClosed, io.EOF:
			seenClosed = true
			return true
		default:
			return false
		}
	}
	return check(err) && seenLoss && (!peerRead || seenInterrupt && seenClosed)
}

func expectedWorkerCancellation(err, expected error) bool {
	if err == nil {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !expectedWorkerCancellation(child, expected) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		if child := wrapped.Unwrap(); child != nil {
			return expectedWorkerCancellation(child, expected)
		}
	}
	return expected != nil && (err == expected || err == context.Canceled)
}

type combinedSourceActivation struct {
	source          wininput.SourceActivation
	capture         wininput.SourceActivation
	observer        peer.ControlFlowObserver
	failureObserver func(error)
}

func (activation *combinedSourceActivation) Activate() error {
	if activation == nil || activation.source == nil || activation.capture == nil {
		return errors.New("combined source activation is not initialized")
	}
	if err := activation.source.Activate(); err != nil {
		activation.observe(peer.ControlFlowSourceSessionActivationFailed)
		failure := fmt.Errorf("%w: %w", ErrSourceSessionActivation, err)
		if activation.failureObserver != nil {
			activation.failureObserver(failure)
		}
		return failure
	}
	activation.observe(peer.ControlFlowSourceSessionActivationSucceeded)
	activation.observe(peer.ControlFlowCaptureActivationAttempted)
	if err := activation.capture.Activate(); err != nil {
		activation.source.Deactivate()
		activation.capture.Deactivate()
		activation.observe(peer.ControlFlowCaptureActivationFailed)
		switch {
		case errors.Is(err, wininput.ErrSourceCaptureActivationAPIRejected):
			activation.observe(peer.ControlFlowCaptureActivationAPIRejected)
		case errors.Is(err, wininput.ErrSourceCaptureActivationVerificationLost):
			activation.observe(peer.ControlFlowCaptureActivationVerificationLost)
		default:
			activation.observe(peer.ControlFlowCaptureActivationSetupFailed)
		}
		failure := fmt.Errorf("%w: %w", ErrSourceCaptureActivation, err)
		if activation.failureObserver != nil {
			activation.failureObserver(failure)
		}
		return failure
	}
	activation.observe(peer.ControlFlowCaptureActivationSucceeded)
	return nil
}

func (activation *combinedSourceActivation) observe(event peer.ControlFlowEvent) {
	if activation != nil && activation.observer != nil {
		activation.observer.ObserveControlFlow(event)
	}
}

func (activation *combinedSourceActivation) Deactivate() {
	if activation == nil {
		return
	}
	if activation.source != nil {
		activation.source.Deactivate()
	}
	if activation.capture != nil {
		activation.capture.Deactivate()
	}
}

func (activation *combinedSourceActivation) Active() bool {
	if activation == nil {
		return false
	}
	return activation.source != nil && activation.source.Active() ||
		activation.capture != nil && activation.capture.Active()
}

type sessionComponentError struct {
	component string
	cause     error
}

func (err *sessionComponentError) Error() string {
	return fmt.Sprintf("%s: %v", err.component, err.cause)
}

func (err *sessionComponentError) Unwrap() error {
	return err.cause
}

func SessionFailureComponent(err error) string {
	var componentErr *sessionComponentError
	if errors.As(err, &componentErr) {
		return componentErr.component
	}
	return "unknown"
}

type observedSourceSink struct {
	delegate wininput.SourceSink
	observer SourceActivityObserver
}

func (sink *observedSourceSink) SendAction(ctx context.Context, action control.InputAction, isCurrent func() bool) error {
	if err := sink.delegate.SendAction(ctx, action, isCurrent); err != nil {
		return err
	}
	sink.observer.ObserveSourceAction(action)
	return nil
}

func (sink *observedSourceSink) Interrupt(cause error) error {
	return sink.delegate.Interrupt(cause)
}

func (sink *observedSourceSink) Fail(ctx context.Context, cause error) error {
	return sink.delegate.Fail(ctx, cause)
}

type component struct {
	name string
	wait <-chan struct{}
	run  func() error
}

func validate(config Config, dependencies Dependencies) error {
	if config.QueueCapacity <= 0 {
		return errors.New("source queue capacity must be positive")
	}
	if config.Now == nil {
		return errors.New("observation clock is required")
	}
	if config.Liveness != nil {
		if err := config.Liveness.Validate(); err != nil {
			return err
		}
	}
	if dependencies.Connection == nil || dependencies.Hook == nil || dependencies.Input == nil || dependencies.Effects == nil || dependencies.Authorizer == nil {
		return errors.New("connection, hook, input, effects, and authorizer are required")
	}
	return nil
}

func DefaultConfig(boundary control.BoundaryConfig) Config {
	return Config{Boundary: boundary, QueueCapacity: 4096, Now: time.Now}
}
