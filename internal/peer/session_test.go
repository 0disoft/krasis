package peer

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestRunSessionRoutesInputAndFailsSafeOnEOF(t *testing.T) {
	runtime, port := newInboundRuntime(t)
	payload, err := transport.EncodeInputPayload(
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		control.InputAction{Kind: control.InputPointerMove, DeltaX: 3, DeltaY: -2},
	)
	if err != nil {
		t.Fatalf("EncodeInputPayload() error = %v", err)
	}
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: payload}}}
	err = RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSession() error = %v, want unexpected EOF", err)
	}
	if len(port.moves) != 1 || port.moves[0] != [2]int32{3, -2} {
		t.Fatalf("moves = %v", port.moves)
	}
	if !connection.closed {
		t.Fatal("connection was not closed")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestRunSessionRoutesControlGrantThenHeartbeat(t *testing.T) {
	runtime := newOutboundRuntime(t)
	grant := controlFrame(t, transport.ControlCommand{
		Kind:      transport.ControlLeaseGrant,
		SessionID: "session-1",
		LeaseID:   "lease-1",
		Epoch:     "epoch-1",
	})
	heartbeatPayload, err := transport.EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	connection := &scriptedConnection{frames: []transport.Frame{
		grant,
		{Kind: transport.MessageHeartbeat, Payload: heartbeatPayload},
	}}
	observedAt := time.Unix(101, 0)
	err = RunSession(context.Background(), connection, runtime, nil, fixedClock(observedAt))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSession() error = %v, want unexpected EOF", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not clean up after EOF: %s", snapshot)
	}
}

func TestRunSessionEOFDisablesOutboundSuppression(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	source := &recordingSourceActivation{active: true}
	controller, err := NewOutboundController(testWriteCoordinator(t, &recordingFrameWriter{}), runtime, source)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	connection := &scriptedConnection{}
	err = RunSessionWithLifecycle(context.Background(), connection, runtime, nil, controller, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSessionWithLifecycle() error = %v", err)
	}
	if source.active {
		t.Fatal("source suppression remained active after EOF")
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestRunSessionClosesConnectionBeforeRuntimeCleanup(t *testing.T) {
	connection := &scriptedConnection{}
	effects := &closeCheckingEffects{connection: connection}
	runtime, err := application.NewSessionRuntime(effects, &inputPort{})
	if err != nil {
		t.Fatalf("NewSessionRuntime() error = %v", err)
	}
	if _, err := runtime.GrantInbound(context.Background(), control.Event{
		Kind: control.EventInboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1",
	}, "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantInbound() error = %v", err)
	}
	effects.calls = 0
	effects.closedForEveryCall = true
	err = RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("RunSession() error = %v, want unexpected EOF", err)
	}
	if effects.calls == 0 || !effects.closedForEveryCall {
		t.Fatalf("cleanup observed open connection: calls=%d all_closed=%t", effects.calls, effects.closedForEveryCall)
	}
}

func TestRunSessionFailsSafeOnMalformedFramePayload(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: []byte{1}}}}
	err := RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, transport.ErrInvalidInputPayload) {
		t.Fatalf("RunSession() error = %v, want ErrInvalidInputPayload", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestRunSessionWrongDirectionInputFailsOutboundRuntime(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	payload, err := transport.EncodeInputPayload(
		control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1},
		control.InputAction{Kind: control.InputPointerMove, DeltaX: 1},
	)
	if err != nil {
		t.Fatalf("EncodeInputPayload() error = %v", err)
	}
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: payload}}}
	err = RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, application.ErrRuntimeNotInbound) {
		t.Fatalf("RunSession() error = %v, want wrong-direction error", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

type scriptedConnection struct {
	frames []transport.Frame
	index  int
	closed bool
}

type closeCheckingEffects struct {
	connection         *scriptedConnection
	calls              int
	closedForEveryCall bool
}

func (effects *closeCheckingEffects) Execute(context.Context, control.Effect) error {
	if effects.calls == 0 {
		effects.closedForEveryCall = true
	}
	effects.calls++
	effects.closedForEveryCall = effects.closedForEveryCall && effects.connection.closed
	return nil
}

func (connection *scriptedConnection) ReadFrame() (transport.Frame, error) {
	if connection.index >= len(connection.frames) {
		return transport.Frame{}, io.EOF
	}
	frame := connection.frames[connection.index]
	connection.index++
	return frame, nil
}

func (connection *scriptedConnection) Close() error {
	connection.closed = true
	return nil
}

func fixedClock(observedAt time.Time) ObservationClock {
	return func() time.Time { return observedAt }
}
