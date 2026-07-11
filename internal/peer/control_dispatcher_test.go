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

func TestDispatchControlGrantActivatesPendingOutbound(t *testing.T) {
	runtime := newOutboundRuntime(t)
	frame := controlFrame(t, transport.ControlCommand{
		Kind:      transport.ControlLeaseGrant,
		SessionID: "session-1",
		LeaseID:   "lease-1",
		Epoch:     "epoch-1",
	})
	if err := DispatchControlFrame(context.Background(), frame, time.Unix(101, 0), runtime, nil); err != nil {
		t.Fatalf("DispatchControlFrame() error = %v", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateOutboundActive {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestDispatchControlRejectReturnsPendingOutboundLocal(t *testing.T) {
	runtime := newOutboundRuntime(t)
	frame := controlFrame(t, transport.ControlCommand{
		Kind:      transport.ControlLeaseReject,
		SessionID: "session-1",
		Epoch:     "epoch-1",
	})
	if err := DispatchControlFrame(context.Background(), frame, time.Unix(101, 0), runtime, nil); err != nil {
		t.Fatalf("DispatchControlFrame() error = %v", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestDispatchControlRevokeReturnsInboundLocal(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	frame := controlFrame(t, transport.ControlCommand{
		Kind:      transport.ControlLeaseRevoke,
		SessionID: "session-1",
		LeaseID:   "lease-1",
		Epoch:     "epoch-1",
	})
	if err := DispatchControlFrame(context.Background(), frame, time.Unix(101, 0), runtime, nil); err != nil {
		t.Fatalf("DispatchControlFrame() error = %v", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("snapshot = %s", snapshot)
	}
}

func TestDispatchControlRequestUsesPolicyHandler(t *testing.T) {
	runtime, err := application.NewSessionRuntime(effectExecutor{}, &inputPort{})
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	requests := &recordingLeaseRequests{}
	command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
	if err := DispatchControlFrame(context.Background(), controlFrame(t, command), time.Unix(101, 0), runtime, requests); err != nil {
		t.Fatalf("DispatchControlFrame() error = %v", err)
	}
	if len(requests.commands) != 1 || requests.commands[0] != command {
		t.Fatalf("requests = %+v", requests.commands)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("request handler unexpectedly mutated runtime: %s", snapshot)
	}
}

func TestDispatchMalformedControlFailsActiveRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	err := DispatchControlFrame(
		context.Background(),
		transport.Frame{Kind: transport.MessageControl, Payload: []byte{1}},
		time.Unix(101, 0),
		runtime,
		nil,
	)
	if !errors.Is(err, transport.ErrInvalidControlPayload) {
		t.Fatalf("error = %v, want ErrInvalidControlPayload", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestDispatchWrongControlFrameKindDoesNotMutateRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	err := DispatchControlFrame(
		context.Background(),
		transport.Frame{Kind: transport.MessageInput},
		time.Unix(101, 0),
		runtime,
		nil,
	)
	if !errors.Is(err, ErrUnexpectedControlFrame) {
		t.Fatalf("error = %v, want ErrUnexpectedControlFrame", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateInboundActive {
		t.Fatalf("runtime changed after router error: %s", snapshot)
	}
}

func newOutboundRuntime(t *testing.T) *application.SessionRuntime {
	t.Helper()
	runtime, err := application.NewSessionRuntime(effectExecutor{}, &inputPort{})
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	if _, err := runtime.BeginOutbound(context.Background(), "session-1", "epoch-1"); err != nil {
		t.Fatalf("BeginOutbound() error = %v", err)
	}
	return runtime
}

func controlFrame(t *testing.T, command transport.ControlCommand) transport.Frame {
	t.Helper()
	payload, err := transport.EncodeControlPayload(command)
	if err != nil {
		t.Fatalf("EncodeControlPayload() error = %v", err)
	}
	return transport.Frame{Kind: transport.MessageControl, Payload: payload}
}

type recordingLeaseRequests struct {
	commands []transport.ControlCommand
}

func (requests *recordingLeaseRequests) HandleLeaseRequest(
	_ context.Context,
	command transport.ControlCommand,
	_ time.Time,
) error {
	requests.commands = append(requests.commands, command)
	return nil
}
