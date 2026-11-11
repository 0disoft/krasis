package peer

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/cleanupresult"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"testing"
	"time"
)

func TestDispatchFailureConfirmsOnlyCompletedCleanup(t *testing.T) {
	for _, failed := range []bool{false, true} {
		var cleanupErr error
		if failed {
			cleanupErr = errors.New("native release failed")
		}
		input := &retryReleaseInput{firstReleaseErr: cleanupErr, alwaysFail: failed}
		runtime, err := application.NewSessionRuntime(effectExecutor{}, input)
		if err != nil {
			t.Fatal(err)
		}
		_, err = runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "session-1", LeaseID: "lease-1"}, "epoch-1", time.Unix(100, 0))
		if err != nil {
			t.Fatal(err)
		}
		if err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}, true); err != nil {
			t.Fatal(err)
		}
		connection := &scriptedConnection{frames: []transport.Frame{{Kind: 255}}}
		err = RunSession(context.Background(), connection, runtime, nil, fixedClock(time.Unix(101, 0)))
		if !errors.Is(err, transport.ErrUnsupportedFrame) || cleanupresult.Confirmed(err) == failed {
			t.Fatalf("failed=%v cleanup evidence=%v: %v", failed, cleanupresult.Confirmed(err), err)
		}
		if failed && !errors.Is(err, cleanupErr) {
			t.Fatalf("lost native release failure: %v", err)
		}
	}
}

func TestSessionDoesNotDispatchQueuedFrameAfterCancellation(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	connection := &scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageInput, Payload: []byte{1}}}}
	err := RunSession(ctx, connection, runtime, nil, fixedClock(time.Unix(101, 0)))
	if !errors.Is(err, context.Canceled) || errors.Is(err, transport.ErrInvalidInputPayload) || !cleanupresult.Confirmed(err) {
		t.Fatalf("dispatched a queued frame after cancellation: %v", err)
	}
}

type disconnectCloseObserver struct {
	scriptedConnection
	beforeClose func()
}

func (c *disconnectCloseObserver) Close() error {
	c.beforeClose()
	return c.scriptedConnection.Close()
}

func TestDisconnectRetiresLeaseBeforeClosingHeartbeatTransport(t *testing.T) {
	runtime, port := newInboundRuntime(t)
	if err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}, true); err != nil {
		t.Fatal(err)
	}
	writer := &recordingFrameWriter{}
	coordinator := testWriteCoordinator(t, writer)
	connection := &disconnectCloseObserver{scriptedConnection: scriptedConnection{frames: []transport.Frame{{Kind: transport.MessageDisconnect}}}}
	connection.beforeClose = func() {
		// Deterministically inject the heartbeat at the former close-before-
		// cleanup race, without depending on scheduling or adding a timeout.
		pulse, err := PulseLiveness(context.Background(), coordinator, runtime, fixedClock(time.Unix(101, 0)), 2*time.Second)
		if err != nil || pulse.Active || pulse.Sent {
			t.Errorf("heartbeat still active at transport close: %+v %v", pulse, err)
		}
	}
	err := RunSessionWithLifecycleObserved(context.Background(), connection, coordinator, runtime, nil, nil, fixedClock(time.Unix(101, 0)), 2*time.Second, nil)
	if !errors.Is(err, ErrPeerDisconnectRequested) {
		t.Fatal(err)
	}
	if snapshot := runtime.Snapshot(); snapshot.PressedCount != 0 || snapshot.Control.State != control.StateLocal || len(port.releases) == 0 {
		t.Fatalf("disconnect did not release input: %+v", snapshot)
	}
}

func TestDisconnectFrameCleansRuntimeAndRejectsPayload(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		runtime, port := newInboundRuntime(t)
		if err := runtime.HandlePressedInput(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}, true); err != nil {
			t.Fatal(err)
		}
		frame := transport.Frame{Kind: transport.MessageDisconnect}
		want := ErrPeerDisconnectRequested
		if malformed {
			frame.Payload = []byte{1}
			want = transport.ErrInvalidFrame
		}
		connection := &scriptedConnection{frames: []transport.Frame{frame}}
		err := RunSessionWithLifecycleObserved(context.Background(), connection, testWriteCoordinator(t, &recordingFrameWriter{}), runtime, nil, nil, fixedClock(time.Unix(101, 0)), 2*time.Second, nil)
		if !errors.Is(err, want) || (malformed && errors.Is(err, ErrPeerDisconnectRequested)) {
			t.Fatalf("malformed=%v: %v", malformed, err)
		}
		if s := runtime.Snapshot(); s.Control.State != control.StateLocal || s.HasLiveness || s.HasStream || s.PressedCount != 0 {
			t.Fatalf("unclean runtime: %+v", s)
		}
		if len(port.releases) == 0 {
			t.Fatal("disconnect left held input")
		}
	}
}
