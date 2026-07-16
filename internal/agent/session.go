package agent

import (
	"context"
	"errors"
	"fmt"
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
	Boundary         control.BoundaryConfig
	QueueCapacity    int
	Now              peer.ObservationClock
	Liveness         *LivenessPolicy
	LivenessObserver peer.LivenessObserver
	SourceObserver   SourceActivityObserver
	PipelineObserver wininput.SourcePipelineObserver
}

type SourceActivityObserver interface {
	ObserveSourceAction(control.InputAction)
	ObserveBoundaryIntent(control.BoundaryIntent)
}

type Dependencies struct {
	Connection Connection
	Hook       SourceHook
	Input      InputBoundaryPort
	Effects    application.EffectExecutor
	Authorizer peer.LeaseAuthorizer
}

type Result struct {
	FinalState control.State
	Cause      error
}

func RunSession(ctx context.Context, config Config, dependencies Dependencies) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("agent context is required")
	}
	if err := validate(config, dependencies); err != nil {
		return Result{}, err
	}
	authenticatedPeer, err := dependencies.Connection.AuthenticatedPeer()
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
	runtime, err := application.NewSessionRuntime(dependencies.Effects, dependencies.Input)
	if err != nil {
		return Result{}, err
	}
	coordinator, err := peer.NewSessionWriteCoordinator(dependencies.Connection)
	if err != nil {
		return Result{}, err
	}
	actionSink, err := peer.NewOutboundActionSink(coordinator, runtime, dependencies.Connection)
	if err != nil {
		return Result{}, err
	}
	var sourceSink wininput.SourceSink = actionSink
	if config.SourceObserver != nil {
		sourceSink = &observedSourceSink{delegate: actionSink, observer: config.SourceObserver}
	}
	source, err := wininput.NewSourceSessionObserved(config.QueueCapacity, sourceSink, config.PipelineObserver)
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
		outboundSource = &combinedSourceActivation{source: source, capture: capture}
	}
	outbound, err := peer.NewOutboundController(coordinator, runtime, outboundSource)
	if err != nil {
		return Result{}, err
	}
	orchestrator, err := peer.NewBoundaryOrchestrator(outbound, peer.NewRandomOutboundIdentity)
	if err != nil {
		return Result{}, err
	}
	responder := &peer.LeaseResponder{
		Coordinator: coordinator, Runtime: runtime, Authorizer: dependencies.Authorizer,
		Peer: authenticatedPeer, Entry: dependencies.Input, Returns: dependencies.Input,
	}
	returns, err := peer.NewReturnController(coordinator, runtime, dependencies.Input)
	if err != nil {
		return Result{}, err
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
			return peer.RunSessionWithLifecycleObserved(runContext, dependencies.Connection, coordinator, runtime, responder, outbound, config.Now, peerTimeout, config.LivenessObserver)
		}},
	}
	if config.Liveness != nil {
		policy := *config.Liveness
		components = append(components, component{name: "peer-liveness", run: func() error {
			return runLiveness(runContext, coordinator, runtime, policy, config.Now, config.LivenessObserver)
		}})
	}

	var workers sync.WaitGroup
	workers.Add(len(components))
	for _, current := range components {
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
			if runErr == nil {
				runErr = fmt.Errorf("%s stopped unexpectedly", current.name)
			}
			if runContext.Err() == nil {
				cancel(&sessionComponentError{component: current.name, cause: runErr})
			}
		}()
	}
	<-runContext.Done()
	workers.Wait()
	cause := context.Cause(runContext)
	snapshot := runtime.Snapshot()
	result := Result{FinalState: snapshot.Control.State, Cause: cause}
	if snapshot.Control.State != control.StateLocal {
		return result, fmt.Errorf("agent session stopped outside LOCAL: %s", snapshot)
	}
	if ctx.Err() != nil && (errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded)) {
		return result, ctx.Err()
	}
	return result, cause
}

type combinedSourceActivation struct {
	source  wininput.SourceActivation
	capture wininput.SourceActivation
}

func (activation *combinedSourceActivation) Activate() error {
	if activation == nil || activation.source == nil || activation.capture == nil {
		return errors.New("combined source activation is not initialized")
	}
	if err := activation.source.Activate(); err != nil {
		return err
	}
	if err := activation.capture.Activate(); err != nil {
		activation.source.Deactivate()
		activation.capture.Deactivate()
		return err
	}
	return nil
}

func (activation *combinedSourceActivation) Deactivate() {
	if activation == nil {
		return
	}
	if activation.capture != nil {
		activation.capture.Deactivate()
	}
	if activation.source != nil {
		activation.source.Deactivate()
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

func (sink *observedSourceSink) SendAction(ctx context.Context, action control.InputAction) error {
	if err := sink.delegate.SendAction(ctx, action); err != nil {
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
