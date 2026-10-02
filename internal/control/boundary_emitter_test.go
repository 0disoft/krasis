package control

import (
	"errors"
	"testing"
	"time"
)

func TestBoundaryIntentEmitterHandsOffWithoutBlocking(t *testing.T) {
	emitter, err := NewBoundaryIntentEmitter(newRightBoundaryDetector(t))
	if err != nil {
		t.Fatalf("NewBoundaryIntentEmitter() error = %v", err)
	}
	if err := emitter.Observe(InputAction{Kind: InputPointerMove, DeltaX: 1}, 99); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	select {
	case intent := <-emitter.Intents():
		if intent.DecidedAt.IsZero() || intent.DecidedAt.After(time.Now()) {
			t.Fatal("consumer did not receive the pre-publication decision time")
		}
		if intent.Edge != ScreenEdgeRight || intent.X != 99 {
			t.Fatalf("intent = %+v", intent)
		}
	default:
		t.Fatal("boundary intent was not queued")
	}
}

func TestBoundaryIntentEmitterPreservesQueuedIntentOnOverflow(t *testing.T) {
	detector := newRightBoundaryDetector(t)
	emitter, err := NewBoundaryIntentEmitter(detector)
	if err != nil {
		t.Fatalf("NewBoundaryIntentEmitter() error = %v", err)
	}
	if err := emitter.Observe(InputAction{Kind: InputPointerMove, DeltaX: 1}, 99); err != nil {
		t.Fatalf("first Observe() error = %v", err)
	}
	if err := emitter.Observe(InputAction{Kind: InputPointerMove, DeltaX: -10}, 89); err != nil {
		t.Fatalf("rearm Observe() error = %v", err)
	}
	err = emitter.Observe(InputAction{Kind: InputPointerMove, DeltaX: 10}, 99)
	if !errors.Is(err, ErrBoundaryIntentQueueFull) {
		t.Fatalf("overflow error = %v", err)
	}
	intent := <-emitter.Intents()
	if intent.X != 99 || intent.Edge != ScreenEdgeRight {
		t.Fatalf("preserved intent = %+v", intent)
	}
}

func TestBoundaryIntentEmitterObservesOnlyQueuedIntents(t *testing.T) {
	detector := newRightBoundaryDetector(t)
	observed := 0
	emitter, err := NewBoundaryIntentEmitterObserved(detector, func(BoundaryIntent) { observed++ })
	if err != nil {
		t.Fatalf("NewBoundaryIntentEmitterObserved() error = %v", err)
	}
	if err := emitter.Observe(InputAction{Kind: InputPointerMove, DeltaX: 1}, 99); err != nil {
		t.Fatalf("first Observe() error = %v", err)
	}
	if err := emitter.Observe(InputAction{Kind: InputPointerMove, DeltaX: -10}, 89); err != nil {
		t.Fatalf("rearm Observe() error = %v", err)
	}
	if err := emitter.Observe(InputAction{Kind: InputPointerMove, DeltaX: 10}, 99); !errors.Is(err, ErrBoundaryIntentQueueFull) {
		t.Fatalf("overflow error = %v", err)
	}
	if observed != 1 {
		t.Fatalf("observed queued intents = %d, want 1", observed)
	}
}

func TestBoundaryIntentEmitterValidatesConstruction(t *testing.T) {
	if _, err := NewBoundaryIntentEmitter(nil); err == nil {
		t.Fatal("NewBoundaryIntentEmitter(nil) error = nil")
	}
	var emitter *BoundaryIntentEmitter
	if err := emitter.Observe(InputAction{Kind: InputPointerMove, DeltaX: 1}, 1); err == nil {
		t.Fatal("nil Observe() error = nil")
	}
	if emitter.Intents() != nil {
		t.Fatal("nil Intents() returned a channel")
	}
}
