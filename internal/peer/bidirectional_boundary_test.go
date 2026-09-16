package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPhysicalBoundaryReversesInboundLeaseInWireOrder(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		runtime, input := newInboundRuntime(t)
		writer := &recordingFrameWriter{}
		coordinator := testWriteCoordinator(t, writer)
		source := &recordingSourceActivation{}
		outbound, _ := NewOutboundController(coordinator, runtime, source)
		returns, _ := NewReturnController(coordinator, runtime, &recordingReturnBoundary{})
		cursor := &recordingReturnCursor{}
		returns.Cursor = cursor
		boundary := &BidirectionalBoundary{Outbound: outbound, Return: returns}
		if !boundary.Ready() {
			t.Fatal("physical reverse crossing blocked")
		}
		if err := boundary.BeginBoundary(context.Background(), control.BoundaryIntent{Edge: edge}, OutboundIdentity{SessionID: "session-2", Epoch: "epoch-2"}); err != nil {
			t.Fatal(err)
		}
		if len(writer.frames) != 2 {
			t.Fatalf("frames=%d", len(writer.frames))
		}
		revoke, _ := transport.DecodeControlPayload(writer.frames[0].Payload)
		request, _ := transport.DecodeControlPayload(writer.frames[1].Payload)
		if revoke.Kind != transport.ControlLeaseRevoke || revoke.SessionID != "session-1" || revoke.LeaseID != "lease-1" || revoke.Epoch != "epoch-1" {
			t.Fatalf("revoke=%+v", revoke)
		}
		if request.Kind != transport.ControlLeaseRequest || request.SessionID != "session-2" || request.Epoch != "epoch-2" || request.Edge != edge {
			t.Fatalf("request=%+v", request)
		}
		if source.active || cursor.prepared != 0 {
			t.Fatal("source activated or idle cursor parked before grant")
		}
		if err := runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}); err != nil {
			t.Fatal("late pending input:", err)
		}
		if err := runtime.ObserveHeartbeat(context.Background(), "session-1", time.Unix(101, 0)); err != nil {
			t.Fatal("late pending heartbeat:", err)
		}
		if err := outbound.GrantPeer(context.Background(), transport.ControlCommand{Kind: transport.ControlLeaseGrant, SessionID: "session-2", LeaseID: "lease-2", Epoch: "epoch-2"}, time.Unix(101, 0)); err != nil {
			t.Fatal(err)
		}
		if !source.active || runtime.Snapshot().Control.State != control.StateOutboundActive {
			t.Fatal("reverse grant did not activate source")
		}
		before := runtime.Snapshot().Liveness
		if err := runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 2}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}); err != nil {
			t.Fatal("late active input:", err)
		}
		if err := runtime.ObserveHeartbeat(context.Background(), "session-1", time.Unix(102, 0)); err != nil {
			t.Fatal("late active heartbeat:", err)
		}
		if len(input.moves) != 0 || runtime.Snapshot().Liveness != before {
			t.Fatal("retired stream affected new lease")
		}
	}
}

func TestPhysicalBoundaryCannotReverseHeldInputOrFailedRevoke(t *testing.T) {
	for _, held := range []bool{true, false} {
		runtime, _ := newInboundRuntime(t)
		writer := &recordingFrameWriter{}
		var port FrameWriter = writer
		if !held {
			port = failedReturnWriter{}
		}
		coordinator := testWriteCoordinator(t, port)
		outbound, _ := NewOutboundController(coordinator, runtime, &recordingSourceActivation{})
		returns, _ := NewReturnController(coordinator, runtime, &recordingReturnBoundary{})
		boundary := &BidirectionalBoundary{Outbound: outbound, Return: returns}
		if held {
			if err := runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.InputAction{Kind: control.InputButtonDown, Code: 1}); err != nil {
				t.Fatal(err)
			}
		}
		err := boundary.BeginBoundary(context.Background(), control.BoundaryIntent{Edge: control.ScreenEdgeLeft}, OutboundIdentity{SessionID: "session-2", Epoch: "epoch-2"})
		if err == nil || (held && !errors.Is(err, ErrOutboundNotReady)) || len(writer.frames) != 0 {
			t.Fatalf("unsafe takeover: %v %+v", err, writer.frames)
		}
	}
}
