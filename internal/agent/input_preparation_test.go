package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestNearBoundaryPreparationIsOneNonblockingHintPerApproach(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		config := control.BoundaryConfig{Edge: edge, ScreenLeft: -100, ScreenWidth: 1000, RearmDistance: 32}
		detector, _ := control.NewBoundaryDetector(config)
		emitter, _ := control.NewBoundaryIntentEmitter(detector)
		p := &preparationBoundary{next: emitter, config: config, hints: make(chan time.Time, 1)}
		near, far, dx := int32(850), int32(400), int32(1)
		if edge == control.ScreenEdgeLeft {
			near, far, dx = -50, 400, -1
		}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: dx}
		if err := p.Observe(move, near); err != nil {
			t.Fatal(err)
		}
		if len(p.hints) != 1 || len(emitter.Intents()) != 0 || !detector.Armed() {
			t.Fatal("hint changed boundary authority")
		}
		<-p.hints
		p.Observe(move, near)
		if len(p.hints) != 0 {
			t.Fatal("duplicate approach created a hint")
		}
		p.Observe(move, far)
		p.Observe(move, near)
		if len(p.hints) != 1 {
			t.Fatal("retreat did not rearm preparation")
		}
		// A full queue drops the next hint and never blocks physical input.
		p.Observe(move, far)
		p.Observe(move, near)
		if len(p.hints) != 1 {
			t.Fatal("hint queue grew")
		}
	}
}

func TestPreparationFreshnessRejectsDelayedOrReversedHint(t *testing.T) {
	now := time.Now()
	for _, delta := range []time.Duration{-time.Millisecond, preparationHintLifetime, time.Second} {
		if freshPreparationHint(now, now.Add(delta)) {
			t.Fatal("stale hint accepted")
		}
	}
	if !freshPreparationHint(now, now.Add(time.Millisecond)) {
		t.Fatal("fresh hint rejected")
	}
}

type preparationTestPort struct{ calls chan struct{} }

func (p preparationTestPort) PrepareInput(ctx context.Context, _ transport.AuthenticatedPeerContext, _ time.Time) {
	select {
	case p.calls <- struct{}{}:
	case <-ctx.Done():
	}
}

func TestPreparationReceiverCoalescesAndNeverGrantsInput(t *testing.T) {
	runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
	ctx, cancel := context.WithCancel(context.Background())
	hints := make(chan time.Time, 3)
	port := preparationTestPort{calls: make(chan struct{}, 3)}
	hints <- time.Now().Add(-time.Second)
	hints <- time.Now()
	done := make(chan error, 1)
	go func() {
		done <- runPreparationReceiver(ctx, hints, runtime, port, transport.AuthenticatedPeerContext{})
	}()
	select {
	case <-port.calls:
	case <-time.After(time.Second):
		t.Fatal("preparation did not run")
	}
	hints <- time.Now()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("preparation did not join cancellation")
	}
	if len(port.calls) != 0 || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatal("preparation granted input or repeated a launch")
	}
}

type preparationFrameWriter struct {
	failure error
	calls   chan transport.Frame
}

func (w preparationFrameWriter) WriteFrameContext(_ context.Context, frame transport.Frame) error {
	w.calls <- frame
	return w.failure
}

func TestPreparationSenderCannotSwallowTLSWriteFailure(t *testing.T) {
	failure := errors.New("write failed")
	writer := preparationFrameWriter{failure: failure, calls: make(chan transport.Frame, 1)}
	coordinator, _ := peer.NewSessionWriteCoordinator(writer)
	runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
	hints := make(chan time.Time, 1)
	hints <- time.Now()
	if err := runPreparationSender(context.Background(), hints, runtime, coordinator); !errors.Is(err, failure) {
		t.Fatal("TLS write failure was hidden")
	}
	frame := <-writer.calls
	command, err := transport.DecodeControlPayload(frame.Payload)
	if err != nil || command != (transport.ControlCommand{Kind: transport.ControlPrepareInput}) || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatal("hint carries lease authority")
	}
}

func TestPreparationSenderDropsHintWhileSemanticGateBusy(t *testing.T) {
	writer := preparationFrameWriter{calls: make(chan transport.Frame, 1)}
	coordinator, _ := peer.NewSessionWriteCoordinator(writer)
	runtime, _ := application.NewSessionRuntime(noopEffects{}, newTestInput(false))
	hints := make(chan time.Time, 1)
	hints <- time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	entered, release, gateDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(gateDone)
		_ = coordinator.Execute(context.Background(), func(peer.FrameWriter) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	defer func() { close(release); <-gateDone }()
	done := make(chan error, 1)
	go func() { done <- runPreparationSender(ctx, hints, runtime, coordinator) }()
	time.Sleep(preparationHintLifetime + 20*time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("expired admission disconnected session: %v", err)
	default:
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not joined")
	}
	if len(writer.calls) != 0 {
		t.Fatal("stale hint was written")
	}
}

var _ wininput.BoundaryObserver = (*preparationBoundary)(nil)

type preparationConnection struct{ Connection }

func (preparationConnection) SupportsInputPreparation() bool { return true }

type preparationHook struct{}

func (preparationHook) RunWithBoundary(ctx context.Context, _ *wininput.SourceSession, boundary wininput.BoundaryObserver, ready func()) error {
	if err := boundary.Observe(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 850); err != nil {
		return err
	}
	ready()
	<-ctx.Done()
	return ctx.Err()
}

type preparationAuthorizer struct {
	acceptingAuthorizer
	preparationTestPort
}

func TestRunSessionRoutesNegotiatedHintWithoutInputGrant(t *testing.T) {
	left, right := openConnectionPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	config := DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 1000, RearmDistance: 32})
	port := preparationTestPort{calls: make(chan struct{}, 1)}
	results := make(chan Result, 2)
	go func() {
		result, _ := RunSession(ctx, config, Dependencies{Connection: &supervisedConnection{Connection: preparationConnection{left}}, Hook: preparationHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
		results <- result
	}()
	go func() {
		result, _ := RunSession(ctx, config, Dependencies{Connection: &supervisedConnection{Connection: preparationConnection{right}}, Hook: &idleHook{}, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: preparationAuthorizer{preparationTestPort: port}})
		results <- result
	}()
	select {
	case <-port.calls:
	case <-ctx.Done():
		t.Fatal("negotiated preparation was not wired to receiver")
	}
	cancel()
	for range 2 {
		select {
		case result := <-results:
			if result.FinalState != control.StateLocal {
				t.Fatal("hint activated remote input")
			}
		case <-time.After(time.Second):
			t.Fatal("preparation prevented session cleanup")
		}
	}
}
