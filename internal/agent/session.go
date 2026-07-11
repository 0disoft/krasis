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
	"github.com/0disoft/krasis/internal/wininput"
)

type SourceHook interface {
	RunWithBoundary(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error
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
}

type Config struct {
	Boundary         control.BoundaryConfig
	QueueCapacity    int
	Now              peer.ObservationClock
	Liveness         *LivenessPolicy
	LivenessObserver peer.LivenessObserver
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
	detector, err := control.NewBoundaryDetector(config.Boundary)
	if err != nil {
		return Result{}, err
	}
	emitter, err := control.NewBoundaryIntentEmitter(detector)
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
	actionSink, err := peer.NewOutboundActionSink(coordinator, runtime)
	if err != nil {
		return Result{}, err
	}
	source, err := wininput.NewSourceSession(config.QueueCapacity, actionSink)
	if err != nil {
		return Result{}, err
	}
	emergency, err := wininput.NewEmergencyBoundaryObserver(source, emitter)
	if err != nil {
		return Result{}, err
	}
	outbound, err := peer.NewOutboundController(coordinator, runtime, source)
	if err != nil {
		return Result{}, err
	}
	orchestrator, err := peer.NewBoundaryOrchestrator(outbound, peer.NewRandomOutboundIdentity)
	if err != nil {
		return Result{}, err
	}
	responder := &peer.LeaseResponder{
		Coordinator: coordinator, Runtime: runtime, Authorizer: dependencies.Authorizer,
		Entry: dependencies.Input, Returns: dependencies.Input,
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
	components := []component{
		{name: "source-session", run: func() error {
			return source.RunWithReady(runContext, func() { close(sourceReady) })
		}},
		{name: "source-hook", wait: sourceReady, run: func() error {
			return dependencies.Hook.RunWithBoundary(runContext, source, emergency, nil)
		}},
		{name: "boundary-orchestrator", wait: sourceReady, run: func() error {
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
				cancel(fmt.Errorf("%s: %w", current.name, runErr))
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
