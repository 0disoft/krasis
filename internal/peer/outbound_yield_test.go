package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestOutboundYieldReleasesExactLeaseAndPermitsAnotherCrossing(t *testing.T) {
	runtime := newOutboundRuntime(t)
	ctx := context.Background()
	if _, err := runtime.GrantOutbound(ctx, "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	writer := &recordingFrameWriter{}
	source := &recordingSourceActivation{active: true}
	controller, _ := NewOutboundController(testWriteCoordinator(t, writer), runtime, source)
	if err := controller.YieldLocal(ctx); err != nil || source.active || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatal("local cleanup failed", err)
	}
	if len(writer.frames) != 1 {
		t.Fatal("expected one revoke", writer.frames)
	}
	command, err := transport.DecodeControlPayload(writer.frames[0].Payload)
	if err != nil || command.Kind != transport.ControlLeaseRevoke || command.SessionID != "session-1" || command.LeaseID != "lease-1" || command.Epoch != "epoch-1" {
		t.Fatal(command, err)
	}
	if err := controller.YieldLocal(ctx); err != nil || len(writer.frames) != 1 {
		t.Fatal("already returned lease was notified twice", err)
	}
	if err := controller.Begin(ctx, "session-2", "epoch-2", control.ScreenEdgeRight); err != nil {
		t.Fatal("connection could not be reused", err)
	}
}

func TestOutboundYieldWriteFailureIsStillFatal(t *testing.T) {
	runtime := newOutboundRuntime(t)
	ctx := context.Background()
	if _, err := runtime.GrantOutbound(ctx, "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	cause := errors.New("peer write failed")
	writer := &recordingFrameWriter{err: cause}
	source := &recordingSourceActivation{active: true}
	controller, _ := NewOutboundController(testWriteCoordinator(t, writer), runtime, source)
	if err := controller.YieldLocal(ctx); !errors.Is(err, cause) || source.active {
		t.Fatal("failed notification accepted", err)
	}
}
