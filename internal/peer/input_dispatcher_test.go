package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestDispatchInputFrameReachesRuntime(t *testing.T) {
	runtime, port := newInboundRuntime(t)
	envelope := control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}
	action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 4, DeltaY: -2}
	payload, err := transport.EncodeInputPayload(envelope, action)
	if err != nil {
		t.Fatalf("EncodeInputPayload() error = %v", err)
	}
	if err := DispatchInputFrame(context.Background(), transport.Frame{Kind: transport.MessageInput, Payload: payload}, runtime); err != nil {
		t.Fatalf("DispatchInputFrame() error = %v", err)
	}
	if len(port.moves) != 1 || port.moves[0] != [2]int32{4, -2} {
		t.Fatalf("moves = %v", port.moves)
	}
}

func TestDispatchMalformedInputFrameFailsRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	err := DispatchInputFrame(context.Background(), transport.Frame{Kind: transport.MessageInput, Payload: []byte{1}}, runtime)
	if !errors.Is(err, transport.ErrInvalidInputPayload) {
		t.Fatalf("error = %v, want ErrInvalidInputPayload", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal || snapshot.HasStream {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestDispatchWrongFrameKindDoesNotMutateRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	err := DispatchInputFrame(context.Background(), transport.Frame{Kind: transport.MessageHeartbeat}, runtime)
	if !errors.Is(err, ErrUnexpectedInputFrame) {
		t.Fatalf("error = %v, want ErrUnexpectedInputFrame", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateInboundActive {
		t.Fatalf("runtime changed after router error: %s", snapshot)
	}
}

func newInboundRuntime(t *testing.T) (*application.SessionRuntime, *inputPort) {
	t.Helper()
	port := &inputPort{}
	runtime, err := application.NewSessionRuntime(effectExecutor{}, port)
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	_, err = runtime.GrantInbound(context.Background(), control.Event{
		Kind:      control.EventInboundLeaseGranted,
		SessionID: "session-1",
		LeaseID:   "lease-1",
	}, "epoch-1", time.Unix(100, 0))
	if err != nil {
		t.Fatalf("GrantInbound() error = %v", err)
	}
	return runtime, port
}

type effectExecutor struct{}

func (effectExecutor) Execute(context.Context, control.Effect) error { return nil }

type inputPort struct {
	moves []([2]int32)
}

func (port *inputPort) Press(context.Context, control.PressedToken) error   { return nil }
func (port *inputPort) Release(context.Context, control.PressedToken) error { return nil }
func (port *inputPort) Move(_ context.Context, x, y int32) error {
	port.moves = append(port.moves, [2]int32{x, y})
	return nil
}
func (port *inputPort) Wheel(context.Context, int32, int32) error { return nil }
