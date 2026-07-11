package peer

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestReturnControllerCleansInboundBeforeWritingExactRevoke(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	writer := &stateRecordingWriter{runtime: runtime}
	returns := &recordingReturnBoundary{}
	controller, err := NewReturnController(writer, runtime, returns)
	if err != nil {
		t.Fatalf("NewReturnController() error = %v", err)
	}
	if err := controller.Revoke(context.Background(), control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if writer.state != control.StateLocal {
		t.Fatalf("runtime state while writing = %s", writer.state)
	}
	command := decodeWrittenControl(t, &writer.recordingFrameWriter)
	if command.Kind != transport.ControlLeaseRevoke || command.SessionID != "session-1" || command.LeaseID != "lease-1" || command.Epoch != "epoch-1" {
		t.Fatalf("revoke = %+v", command)
	}
	if len(returns.disarmed) != 1 {
		t.Fatalf("disarmed = %+v", returns.disarmed)
	}
}

func TestReturnControllerIgnoresStaleQueuedIntent(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	controller, _ := NewReturnController(&recordingFrameWriter{}, runtime, &recordingReturnBoundary{})
	intents := make(chan control.ReturnIntent, 2)
	intents <- control.ReturnIntent{SessionID: "stale", Epoch: "stale"}
	intents <- control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}
	close(intents)
	if err := controller.Run(context.Background(), intents); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime = %s", snapshot)
	}
}

func TestReturnControllerDoesNotWriteWhenInputIsHeld(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	if err := runtime.HandleInputAction(
		context.Background(),
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		control.InputAction{Kind: control.InputButtonDown, Code: 1},
	); err != nil {
		t.Fatalf("HandleInputAction() error = %v", err)
	}
	writer := &recordingFrameWriter{}
	returns := &recordingReturnBoundary{}
	controller, _ := NewReturnController(writer, runtime, returns)
	err := controller.Revoke(context.Background(), control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"})
	if !errors.Is(err, application.ErrRuntimeInputHeld) || len(writer.frames) != 0 {
		t.Fatalf("Revoke(held) error=%v frames=%d", err, len(writer.frames))
	}
	if len(returns.rearmed) != 1 {
		t.Fatalf("rearmed = %+v", returns.rearmed)
	}
}

func TestReturnControllerRunContinuesAfterHeldInputRace(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	if err := runtime.HandleInputAction(
		context.Background(),
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		control.InputAction{Kind: control.InputButtonDown, Code: 1},
	); err != nil {
		t.Fatalf("HandleInputAction(down) error = %v", err)
	}
	returns := &recordingReturnBoundary{}
	controller, _ := NewReturnController(&recordingFrameWriter{}, runtime, returns)
	intents := make(chan control.ReturnIntent, 1)
	intents <- control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}
	close(intents)
	if err := controller.Run(context.Background(), intents); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(returns.rearmed) != 1 || runtime.Snapshot().Control.State != control.StateInboundActive {
		t.Fatalf("rearmed=%+v runtime=%s", returns.rearmed, runtime.Snapshot())
	}
}

type recordingReturnBoundary struct {
	armed    []control.ReturnIntent
	rearmed  []control.ReturnIntent
	disarmed []control.ReturnIntent
}

func (boundary *recordingReturnBoundary) ArmReturn(intent control.ReturnIntent, _ control.ScreenEdge) error {
	boundary.armed = append(boundary.armed, intent)
	return nil
}

func (boundary *recordingReturnBoundary) RearmReturn(intent control.ReturnIntent) {
	boundary.rearmed = append(boundary.rearmed, intent)
}

func (boundary *recordingReturnBoundary) DisarmReturn(intent control.ReturnIntent) {
	boundary.disarmed = append(boundary.disarmed, intent)
}

type stateRecordingWriter struct {
	recordingFrameWriter
	runtime *application.SessionRuntime
	state   control.State
}

func (writer *stateRecordingWriter) WriteFrameContext(ctx context.Context, frame transport.Frame) error {
	writer.state = writer.runtime.Snapshot().Control.State
	return writer.recordingFrameWriter.WriteFrameContext(ctx, frame)
}
