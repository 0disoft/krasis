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

// ADR 0020 must hold through the product receiver with a nonzero timeout, not
// just when ObserveHeartbeat is called directly on the runtime.
func TestRunSessionObservedDrainsRetiredHeartbeat(t *testing.T) {
	for _, timeout := range []time.Duration{0, 2 * time.Second} {
		t.Run(timeout.String(), func(t *testing.T) {
			runtime, port := newInboundRuntime(t)
			if _, _, err := runtime.RevokeLocalReturn(context.Background(), control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}); err != nil {
				t.Fatal(err)
			}
			payload, err := transport.EncodeHeartbeatPayload("session-1")
			if err != nil {
				t.Fatal(err)
			}
			connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageHeartbeat, Payload: payload}}}
			observer := &sessionLivenessObserver{}
			err = RunSessionWithLifecycleObserved(context.Background(), connection, testWriteCoordinator(t, &recordingFrameWriter{}), runtime, nil, nil, fixedClock(time.Unix(101, 0)), timeout, observer)
			if !errors.Is(err, ErrPeerRemoteEOF) || errors.Is(err, ErrPeerHeartbeatDispatch) {
				t.Fatalf("retired heartbeat must drain before scripted EOF: %v", err)
			}
			if len(observer.received) != 1 {
				t.Fatal("retired heartbeat did not reach dispatcher")
			}
			if s := runtime.Snapshot(); s.Control.State != control.StateLocal || s.HasLiveness || s.HasStream || s.PressedCount != 0 {
				t.Fatalf("retired heartbeat reactivated runtime: %+v", s)
			}
			if len(port.presses) != 0 || len(port.moves) != 0 {
				t.Fatal("retired heartbeat injected input")
			}
		})
	}
}

func TestRunSessionObservedRejectsInactiveHeartbeat(t *testing.T) {
	for _, retired := range []bool{false, true} {
		t.Run(map[bool]string{false: "no retired identity", true: "unrelated identity"}[retired], func(t *testing.T) {
			var runtime *application.SessionRuntime
			if retired {
				runtime, _ = newInboundRuntime(t)
				if _, _, err := runtime.RevokeLocalReturn(context.Background(), control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}); err != nil {
					t.Fatal(err)
				}
			} else {
				var err error
				runtime, err = application.NewSessionRuntime(effectExecutor{}, &inputPort{})
				if err != nil {
					t.Fatal(err)
				}
			}
			payload, err := transport.EncodeHeartbeatPayload("session-other")
			if err != nil {
				t.Fatal(err)
			}
			connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageHeartbeat, Payload: payload}}}
			observer := &sessionLivenessObserver{}
			err = RunSessionWithLifecycleObserved(context.Background(), connection, testWriteCoordinator(t, &recordingFrameWriter{}), runtime, nil, nil, fixedClock(time.Unix(101, 0)), 2*time.Second, observer)
			if !errors.Is(err, application.ErrRuntimeNoLiveness) || !errors.Is(err, ErrPeerHeartbeatDispatch) || len(observer.received) != 0 {
				t.Fatalf("invalid heartbeat accepted: %v", err)
			}
			if s := runtime.Snapshot(); s.Control.State != control.StateLocal || s.HasLiveness {
				t.Fatal("inactive runtime reactivated")
			}
		})
	}
}
