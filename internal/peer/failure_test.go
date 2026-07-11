package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestFailRuntimeCoordinatedPreservesCauseWhenCoordinatorTimesOut(t *testing.T) {
	runtime := newOutboundRuntime(t)
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	held := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- coordinator.Execute(context.Background(), func(FrameWriter) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	cause := errors.New("connection failed")
	err := failRuntimeCoordinatedWithin(
		context.Background(), coordinator, runtime, cause, nil, 10*time.Millisecond,
	)
	if !errors.Is(err, cause) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("failRuntimeCoordinatedWithin() error = %v, want cause and deadline", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Control.State != control.StatePending {
		t.Fatalf("timed-out cleanup changed runtime outside coordinator: %s", snapshot)
	}

	close(release)
	select {
	case err := <-holderDone:
		if err != nil {
			t.Fatalf("held coordinator operation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("held coordinator operation did not finish")
	}
}
