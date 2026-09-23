package peer

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestBoundaryOrchestratorStartsOutboundRequest(t *testing.T) {
	beginner := &recordingBoundaryBeginner{ready: true}
	wantIdentity := OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}
	orchestrator, err := NewBoundaryOrchestrator(beginner, func() (OutboundIdentity, error) {
		return wantIdentity, nil
	})
	if err != nil {
		t.Fatalf("NewBoundaryOrchestrator() error = %v", err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	wantIntent := control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 1919}
	intents <- wantIntent
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if beginner.calls != 1 || beginner.intent != wantIntent || beginner.identity != wantIdentity {
		t.Fatalf("beginner = %+v", beginner)
	}
}

func TestBoundaryOrchestratorDropsIntentWhileOutboundIsNotReady(t *testing.T) {
	beginner := &recordingBoundaryBeginner{}
	identityCalls := 0
	orchestrator, err := NewBoundaryOrchestrator(beginner, func() (OutboundIdentity, error) {
		identityCalls++
		return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
	})
	if err != nil {
		t.Fatalf("NewBoundaryOrchestrator() error = %v", err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 99}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if beginner.calls != 0 || identityCalls != 0 {
		t.Fatalf("begin calls = %d, identity calls = %d", beginner.calls, identityCalls)
	}
}

func TestBoundaryIntentObservationDoesNotBlockHookHandoff(t *testing.T) {
	detector, err := control.NewBoundaryDetector(control.BoundaryConfig{
		Edge: control.ScreenEdgeRight, ScreenWidth: 100, ActivationGap: 1, RearmDistance: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	emitter, err := control.NewBoundaryIntentEmitter(detector)
	if err != nil {
		t.Fatal(err)
	}
	orchestrator, err := NewBoundaryOrchestrator(&recordingBoundaryBeginner{}, func() (OutboundIdentity, error) {
		return OutboundIdentity{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	var observed atomic.Int32
	go func() {
		done <- orchestrator.RunWithIntentObserver(ctx, emitter.Intents(), func(control.BoundaryIntent) {
			if observed.Add(1) == 1 {
				close(entered)
				<-release
			}
		})
	}()
	if err := emitter.Observe(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 99); err != nil {
		close(release)
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("orchestrator did not observe queued intent")
	}
	if err := emitter.Observe(control.InputAction{Kind: control.InputPointerMove, DeltaX: -12}, 87); err != nil {
		close(release)
		t.Fatal(err)
	}
	if err := emitter.Observe(control.InputAction{Kind: control.InputPointerMove, DeltaX: 12}, 99); err != nil {
		close(release)
		t.Fatalf("hook handoff blocked by diagnostic observer: %v", err)
	}
	close(release)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("orchestrator did not stop")
	}
}

func TestBoundaryIntentObserverPanicFailsSession(t *testing.T) {
	orchestrator, err := NewBoundaryOrchestrator(&recordingBoundaryBeginner{ready: true}, func() (OutboundIdentity, error) {
		return OutboundIdentity{SessionID: "s", Epoch: "e"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeRight, X: 99}
	close(intents)
	err = orchestrator.RunWithIntentObserver(context.Background(), intents, func(control.BoundaryIntent) {
		panic("private input")
	})
	if !errors.Is(err, ErrBoundaryIntentObserverPanic) || err.Error() != ErrBoundaryIntentObserverPanic.Error() {
		t.Fatalf("observer panic returned %v", err)
	}
}

func TestBoundaryOrchestratorReturnsIdentityAndBeginFailures(t *testing.T) {
	cause := errors.New("entropy unavailable")
	orchestrator, err := NewBoundaryOrchestrator(&recordingBoundaryBeginner{ready: true}, func() (OutboundIdentity, error) {
		return OutboundIdentity{}, cause
	})
	if err != nil {
		t.Fatalf("NewBoundaryOrchestrator() error = %v", err)
	}
	intents := make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v", err)
	}

	beginCause := errors.New("write failed")
	orchestrator, _ = NewBoundaryOrchestrator(&recordingBoundaryBeginner{ready: true, err: beginCause}, func() (OutboundIdentity, error) {
		return OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}, nil
	})
	intents = make(chan control.BoundaryIntent, 1)
	intents <- control.BoundaryIntent{Edge: control.ScreenEdgeLeft}
	close(intents)
	if err := orchestrator.Run(context.Background(), intents); !errors.Is(err, beginCause) {
		t.Fatalf("Run(begin failure) error = %v", err)
	}
}

func TestRandomOutboundIdentityUsesIndependent128BitValues(t *testing.T) {
	input := make([]byte, outboundIdentifierBytes*2)
	for index := range input {
		input[index] = byte(index)
	}
	identity, err := randomOutboundIdentity(bytes.NewReader(input))
	if err != nil {
		t.Fatalf("randomOutboundIdentity() error = %v", err)
	}
	if identity.SessionID != "000102030405060708090a0b0c0d0e0f" {
		t.Fatalf("session ID = %q", identity.SessionID)
	}
	if identity.Epoch != "101112131415161718191a1b1c1d1e1f" {
		t.Fatalf("epoch = %q", identity.Epoch)
	}
}

type recordingBoundaryBeginner struct {
	ready    bool
	calls    int
	intent   control.BoundaryIntent
	identity OutboundIdentity
	err      error
}

func (beginner *recordingBoundaryBeginner) Ready() bool { return beginner.ready }

func (beginner *recordingBoundaryBeginner) BeginBoundary(
	_ context.Context,
	intent control.BoundaryIntent,
	identity OutboundIdentity,
) error {
	beginner.calls++
	beginner.intent = intent
	beginner.identity = identity
	return beginner.err
}
