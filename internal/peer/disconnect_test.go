package peer

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"testing"
	"time"
)

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
