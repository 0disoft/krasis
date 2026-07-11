package wininput

import (
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestReturnBoundaryEmitsExactSessionAtDestinationEntryEdge(t *testing.T) {
	tests := []struct {
		source   control.ScreenEdge
		pointerX int32
	}{
		{source: control.ScreenEdgeRight, pointerX: -1920},
		{source: control.ScreenEdgeLeft, pointerX: -1},
	}
	for _, test := range tests {
		adapter := newInputAdapter()
		intent := control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}
		if err := adapter.ArmReturn(intent, test.source); err != nil {
			t.Fatalf("ArmReturn(%s) error = %v", test.source, err)
		}
		if err := adapter.observeReturnPosition(test.pointerX, -1920, 1920); err != nil {
			t.Fatalf("observeReturnPosition(%s) error = %v", test.source, err)
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
	if err := adapter.observeReturnPosition(0, 0, 100); err != nil {
		t.Fatalf("observeReturnPosition(old) error = %v", err)
	}
	if err := adapter.ArmReturn(current, control.ScreenEdgeRight); err != nil {
		t.Fatalf("ArmReturn(current) error = %v", err)
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
