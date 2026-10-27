package peer

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPeerRevokeDeadlineReleasesWriterWhileNativeInputIsJoined(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &laneInput{entered: make(chan struct{}), resume: make(chan struct{})}
		runtime, err := application.NewSessionRuntime(effectExecutor{}, input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.GrantInbound(context.Background(), control.Event{Kind: control.EventInboundLeaseGranted, SessionID: "s", LeaseID: "l"}, "e", time.Now()); err != nil {
			t.Fatal(err)
		}
		inputDone := make(chan error, 1)
		go func() {
			inputDone <- runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "s", Epoch: "e", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 30})
		}()
		<-input.entered
		go func() { time.Sleep(500 * time.Millisecond); close(input.resume) }()
		defer func() {
			if err := <-inputDone; err != nil {
				t.Error("native operation did not complete", err)
			}
			if err := runtime.Fail(context.Background(), context.Canceled); !errors.Is(err, context.Canceled) {
				t.Error("fixture cleanup failed", err)
			}
		}()
		writer := &recordingFrameWriter{}
		coordinator := testWriteCoordinator(t, writer)
		controller, err := NewOutboundController(coordinator, runtime, &recordingSourceActivation{})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()
		started := time.Now()
		err = controller.RevokePeer(ctx, transport.ControlCommand{Kind: transport.ControlLeaseRevoke, SessionID: "s", LeaseID: "l", Epoch: "e"})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 25*time.Millisecond {
			t.Fatalf("revoke ignored admission deadline: err=%v wait=%v", err, time.Since(started))
		}
		select {
		case <-input.resume:
			t.Fatal("revocation fabricated native retirement")
		default:
		}
		pulse, err := PulseLiveness(context.Background(), coordinator, runtime, time.Now, 2*time.Second)
		if err != nil || !pulse.Sent || len(writer.frames) != 1 {
			t.Fatal("expired revoke retained writer or changed lease", pulse, err)
		}
	})
}
