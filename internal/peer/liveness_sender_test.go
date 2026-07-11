package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPulseLivenessSkipsInactiveRuntime(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	active, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, time.Unix(101, 0), 3*time.Second,
	)
	if err != nil || active || len(writer.frames) != 0 {
		t.Fatalf("PulseLiveness(LOCAL) = active %t frames %d error %v", active, len(writer.frames), err)
	}
}

func TestPulseLivenessWritesCurrentSessionHeartbeat(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	writer := &recordingFrameWriter{}
	active, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, time.Unix(101, 0), 3*time.Second,
	)
	if err != nil || !active || len(writer.frames) != 1 || writer.frames[0].Kind != transport.MessageHeartbeat {
		t.Fatalf("PulseLiveness(active) = active %t frames %+v error %v", active, writer.frames, err)
	}
	sessionID, err := transport.DecodeHeartbeatPayload(writer.frames[0].Payload)
	if err != nil || sessionID != "session-1" {
		t.Fatalf("heartbeat session = %q error %v", sessionID, err)
	}
}

func TestPulseLivenessExpiresWithoutWritingHeartbeat(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	writer := &recordingFrameWriter{}
	active, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, time.Unix(103, 0), 3*time.Second,
	)
	if !active || !errors.Is(err, ErrPeerLivenessExpired) || len(writer.frames) != 0 {
		t.Fatalf("PulseLiveness(expired) = active %t frames %+v error %v", active, writer.frames, err)
	}
	if state := runtime.Snapshot().Control.State; state != control.StateLocal {
		t.Fatalf("expired runtime = %s", runtime.Snapshot())
	}
}

func TestPulseLivenessWriteFailureCleansRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	cause := errors.New("heartbeat write failed")
	writer := &recordingFrameWriter{err: cause}
	active, err := PulseLiveness(
		context.Background(), testWriteCoordinator(t, writer), runtime, time.Unix(101, 0), 3*time.Second,
	)
	if !active || !errors.Is(err, cause) {
		t.Fatalf("PulseLiveness(write failure) = active %t error %v", active, err)
	}
	if state := runtime.Snapshot().Control.State; state != control.StateLocal {
		t.Fatalf("write failure runtime = %s", runtime.Snapshot())
	}
}
