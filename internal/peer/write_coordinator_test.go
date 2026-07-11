package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestSessionWriteCoordinatorSerializesOperations(t *testing.T) {
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- coordinator.Execute(context.Background(), func(FrameWriter) error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	<-firstEntered

	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- coordinator.Execute(context.Background(), func(FrameWriter) error {
			close(secondEntered)
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Fatal("second semantic write entered before the first completed")
	case <-time.After(25 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
}

func TestSessionWriteCoordinatorCanceledWaiterDoesNotExecute(t *testing.T) {
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- coordinator.Execute(context.Background(), func(FrameWriter) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executed := false
	err := coordinator.Execute(ctx, func(FrameWriter) error {
		executed = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || executed {
		t.Fatalf("canceled Execute() = error %v executed %t", err, executed)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("held Execute() error = %v", err)
	}
}

func TestSessionWriteCoordinatorOrdersHeartbeatBeforeRevokeAndDropsLateHeartbeat(t *testing.T) {
	runtime, _ := newInboundRuntime(t)
	writer := &recordingFrameWriter{}
	coordinator := testWriteCoordinator(t, writer)
	returns := &recordingReturnBoundary{}
	controller, err := NewReturnController(coordinator, runtime, returns)
	if err != nil {
		t.Fatalf("NewReturnController() error = %v", err)
	}
	if err := SendHeartbeat(context.Background(), coordinator, runtime); err != nil {
		t.Fatalf("SendHeartbeat(active) error = %v", err)
	}
	if err := controller.Revoke(context.Background(), control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if len(writer.frames) != 2 || writer.frames[0].Kind != transport.MessageHeartbeat || writer.frames[1].Kind != transport.MessageControl {
		t.Fatalf("semantic frame order = %+v", writer.frames)
	}
	if err := SendHeartbeat(context.Background(), coordinator, runtime); err == nil {
		t.Fatal("SendHeartbeat(LOCAL) error = nil")
	}
	if len(writer.frames) != 2 {
		t.Fatalf("late heartbeat wrote a frame: %+v", writer.frames)
	}
}

func TestOutboundControllerFailUsesCleanupContextAfterCancellation(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	source := &recordingSourceActivation{active: true}
	controller, err := NewOutboundController(testWriteCoordinator(t, &recordingFrameWriter{}), runtime, source)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cause := errors.New("connection stopped")
	if err := controller.Fail(ctx, cause); !errors.Is(err, cause) {
		t.Fatalf("Fail(canceled context) error = %v", err)
	}
	if source.Active() || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("canceled cleanup left active state: source=%t runtime=%s", source.Active(), runtime.Snapshot())
	}
}

func TestOutboundControllerFailDeactivatesAgainInsideCoordinator(t *testing.T) {
	runtime := newOutboundRuntime(t)
	if _, err := runtime.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatalf("GrantOutbound() error = %v", err)
	}
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	source := newIntegrationSourceActivation()
	if err := source.Activate(); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	<-source.activated
	controller, err := NewOutboundController(coordinator, runtime, source)
	if err != nil {
		t.Fatalf("NewOutboundController() error = %v", err)
	}

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
	cause := errors.New("session failed")
	failDone := make(chan error, 1)
	go func() { failDone <- controller.Fail(context.Background(), cause) }()
	waitSignal(t, source.deactivated, "immediate source deactivation")
	if err := source.Activate(); err != nil {
		t.Fatalf("reactivate during cleanup wait: %v", err)
	}
	<-source.activated
	close(release)
	if err := <-holderDone; err != nil {
		t.Fatalf("held Execute() error = %v", err)
	}
	if err := <-failDone; !errors.Is(err, cause) {
		t.Fatalf("Fail() error = %v", err)
	}
	waitSignal(t, source.deactivated, "coordinated source deactivation")
	if source.Active() || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("cleanup left active state: source=%t runtime=%s", source.Active(), runtime.Snapshot())
	}
}
