package wininput

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestReturnBoundaryEmitsExactSessionAtDestinationEntryEdge(t *testing.T) {
	tests := []struct {
		source    control.ScreenEdge
		inwardX   int32
		boundaryX int32
	}{
		{source: control.ScreenEdgeRight, inwardX: -1910, boundaryX: -1920},
		{source: control.ScreenEdgeLeft, inwardX: -11, boundaryX: -1},
	}
	for _, test := range tests {
		adapter, err := newInputAdapterWithReturnRearmDistance(10)
		if err != nil {
			t.Fatal(err)
		}
		intent := control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}
		if err := adapter.ArmReturn(intent, test.source); err != nil {
			t.Fatalf("ArmReturn(%s) error = %v", test.source, err)
		}
		if err := adapter.observeReturnPosition(test.boundaryX, -1920, 1920); err != nil {
			t.Fatalf("observeReturnPosition(initial boundary %s) error = %v", test.source, err)
		}
		select {
		case <-adapter.ReturnIntents():
			t.Fatalf("return intent for %s fired before inward hysteresis", test.source)
		default:
		}
		if err := adapter.observeReturnPosition(test.inwardX, -1920, 1920); err != nil {
			t.Fatalf("observeReturnPosition(inward %s) error = %v", test.source, err)
		}
		if err := adapter.observeReturnPosition(test.boundaryX, -1920, 1920); err != nil {
			t.Fatalf("observeReturnPosition(return %s) error = %v", test.source, err)
		}
		select {
		case got := <-adapter.ReturnIntents():
			if got != intent {
				t.Fatalf("intent = %+v", got)
			}
		default:
			t.Fatalf("return intent for %s was not emitted", test.source)
		}
	}
}

func TestReturnBoundaryBlocksWhileInputIsPressed(t *testing.T) {
	adapter := newInputAdapter()
	intent := control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}
	if err := adapter.ArmReturn(intent, control.ScreenEdgeRight); err != nil {
		t.Fatalf("ArmReturn() error = %v", err)
	}
	if err := adapter.observeReturnPosition(defaultReturnRearmDistance, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(inward) error = %v", err)
	}
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 1}
	if err := adapter.recordReturnInput(token, true); err != nil {
		t.Fatalf("recordReturnInput(down) error = %v", err)
	}
	if err := adapter.observeReturnPosition(0, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(held) error = %v", err)
	}
	select {
	case <-adapter.ReturnIntents():
		t.Fatal("held input emitted return intent")
	default:
	}
	if err := adapter.recordReturnInput(token, false); err != nil {
		t.Fatalf("recordReturnInput(up) error = %v", err)
	}
	if err := adapter.observeReturnPosition(0, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(released) error = %v", err)
	}
	select {
	case <-adapter.ReturnIntents():
	default:
		t.Fatal("released input did not emit return intent")
	}
}

func TestReturnBoundaryNewArmDropsStaleQueuedIntent(t *testing.T) {
	adapter := newInputAdapter()
	old := control.ReturnIntent{SessionID: "old", Epoch: "old"}
	current := control.ReturnIntent{SessionID: "current", Epoch: "current"}
	if err := adapter.ArmReturn(old, control.ScreenEdgeRight); err != nil {
		t.Fatalf("ArmReturn(old) error = %v", err)
	}
	if err := adapter.observeReturnPosition(defaultReturnRearmDistance, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(old inward) error = %v", err)
	}
	if err := adapter.observeReturnPosition(0, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(old) error = %v", err)
	}
	if err := adapter.ArmReturn(current, control.ScreenEdgeRight); err != nil {
		t.Fatalf("ArmReturn(current) error = %v", err)
	}
	if err := adapter.observeReturnPosition(defaultReturnRearmDistance, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(current inward) error = %v", err)
	}
	if err := adapter.observeReturnPosition(0, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(current) error = %v", err)
	}
	if got := <-adapter.ReturnIntents(); got != current {
		t.Fatalf("intent = %+v, want current", got)
	}
}

func TestReturnBoundaryRearmsOnlyMatchingSession(t *testing.T) {
	adapter := newInputAdapter()
	intent := control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}
	if err := adapter.ArmReturn(intent, control.ScreenEdgeRight); err != nil {
		t.Fatalf("ArmReturn() error = %v", err)
	}
	if err := adapter.observeReturnPosition(defaultReturnRearmDistance, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(inward) error = %v", err)
	}
	if err := adapter.observeReturnPosition(0, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition() error = %v", err)
	}
	<-adapter.ReturnIntents()
	adapter.RearmReturn(control.ReturnIntent{SessionID: "stale", Epoch: "stale"})
	if err := adapter.observeReturnPosition(0, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(stale rearm) error = %v", err)
	}
	select {
	case <-adapter.ReturnIntents():
		t.Fatal("stale session rearmed return detector")
	default:
	}
	adapter.RearmReturn(intent)
	if err := adapter.observeReturnPosition(0, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(rearm) error = %v", err)
	}
	select {
	case <-adapter.ReturnIntents():
	default:
		t.Fatal("matching session did not rearm return detector")
	}
}

func TestReturnBoundaryRejectsUnsafeRearmDistance(t *testing.T) {
	if _, err := NewInputAdapterWithReturnRearmDistance(cursorEntryInset, 1920); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("unsafe return rearm distance error = %v", err)
	}
	if _, err := NewInputAdapterWithReturnRearmDistance(1920, 1920); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("screen-width return rearm distance error = %v", err)
	}
	if _, err := NewInputAdapterWithReturnRearmDistance(1921, 1920); !errors.Is(err, ErrUnsupportedInput) {
		t.Fatalf("oversized return rearm distance error = %v", err)
	}
}
