package peer

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestOrderedInputRenewsLeaseWhileHeartbeatIsQueued(t *testing.T) {
	runtime, port := newInboundRuntime(t)
	connection := &scriptedConnection{}
	for sequence := uint64(1); sequence <= 4; sequence++ {
		payload, err := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: sequence}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
		if err != nil {
			t.Fatal(err)
		}
		connection.frames = append(connection.frames, transport.Frame{Kind: transport.MessageInput, Payload: payload})
	}
	heartbeat, _ := transport.EncodeHeartbeatPayload("session-1")
	connection.frames = append(connection.frames, transport.Frame{Kind: transport.MessageHeartbeat, Payload: heartbeat})
	observedAt := time.Unix(100, 0)
	observer := &sessionLivenessObserver{}
	err := runSession(context.Background(), connection, testWriteCoordinator(t, &recordingFrameWriter{}), runtime, nil, nil, func() time.Time {
		observedAt = observedAt.Add(time.Second)
		return observedAt
	}, 2*time.Second, observer)
	if !errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, ErrPeerLivenessExpired) || len(port.moves) != 4 || len(observer.received) != 1 {
		t.Fatalf("active ordered stream timed out: %v moves=%d heartbeats=%d", err, len(port.moves), len(observer.received))
	}
}

func TestOrderedInputCannotReviveExpiredLease(t *testing.T) {
	runtime, port := newInboundRuntime(t)
	payload, _ := transport.EncodeInputPayload(control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: payload}}}
	err := runSession(context.Background(), connection, nil, runtime, nil, nil, fixedClock(time.Unix(102, 0)), 2*time.Second, nil)
	if !errors.Is(err, ErrPeerLivenessExpired) || len(port.moves) != 0 || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatal("expired frame renewed lease", err)
	}
}
