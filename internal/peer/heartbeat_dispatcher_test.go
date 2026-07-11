package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestDispatchHeartbeatFrameUpdatesLocalObservation(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	payload, err := transport.EncodeHeartbeatPayload("session-1")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	observedAt := time.Unix(101, 0)
	observation, err := DispatchHeartbeatFrame(context.Background(), transport.Frame{Kind: transport.MessageHeartbeat, Payload: payload}, observedAt, runtime)
	if err != nil {
		t.Fatalf("DispatchHeartbeatFrame() error = %v", err)
	}
	if observation.SessionID != "session-1" || !observation.ObservedAt.Equal(observedAt) {
		t.Fatalf("observation = %+v", observation)
	}
	if snapshot := runtime.Snapshot(); !snapshot.Liveness.LastObserved.Equal(observedAt) {
		t.Fatalf("last observation = %v", snapshot.Liveness.LastObserved)
	}
}

func TestDispatchMalformedHeartbeatFailsRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	_, err := DispatchHeartbeatFrame(
		context.Background(),
		transport.Frame{Kind: transport.MessageHeartbeat, Payload: []byte{1}},
		time.Unix(101, 0),
		runtime,
	)
	if !errors.Is(err, transport.ErrInvalidHeartbeatPayload) {
		t.Fatalf("error = %v, want ErrInvalidHeartbeatPayload", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestDispatchStaleHeartbeatFailsRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	payload, err := transport.EncodeHeartbeatPayload("old-session")
	if err != nil {
		t.Fatalf("EncodeHeartbeatPayload() error = %v", err)
	}
	_, err = DispatchHeartbeatFrame(
		context.Background(),
		transport.Frame{Kind: transport.MessageHeartbeat, Payload: payload},
		time.Unix(101, 0),
		runtime,
	)
	if !errors.Is(err, control.ErrStaleHeartbeat) {
		t.Fatalf("error = %v, want ErrStaleHeartbeat", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateLocal {
		t.Fatalf("runtime did not fail safe: %s", snapshot)
	}
}

func TestDispatchWrongHeartbeatFrameKindDoesNotMutateRuntime(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	_, err := DispatchHeartbeatFrame(
		context.Background(),
		transport.Frame{Kind: transport.MessageInput},
		time.Unix(101, 0),
		runtime,
	)
	if !errors.Is(err, ErrUnexpectedHeartbeatFrame) {
		t.Fatalf("error = %v, want ErrUnexpectedHeartbeatFrame", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StateInboundActive {
		t.Fatalf("runtime changed after router error: %s", snapshot)
	}
}
