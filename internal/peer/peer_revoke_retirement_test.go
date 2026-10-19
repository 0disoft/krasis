package peer

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type revokeRetirementBoundary struct {
	recordingReturnBoundary
	disarmed []control.ReturnIntent
}

func (boundary *revokeRetirementBoundary) DisarmReturn(intent control.ReturnIntent) {
	boundary.disarmed = append(boundary.disarmed, intent)
}

func TestPeerRevokeDisarmsOnlySuccessfullyRevokedInboundLease(t *testing.T) {
	for _, direction := range []string{"inbound", "outbound", "stale"} {
		t.Run(direction, func(t *testing.T) {
			ctx := context.Background()
			runtime, _ := newInboundRuntime(t)
			if direction == "outbound" {
				runtime = newOutboundRuntime(t)
				if _, err := runtime.GrantOutbound(ctx, "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
					t.Fatal(err)
				}
			}
			boundary := &revokeRetirementBoundary{}
			writer := &recordingFrameWriter{}
			controller, err := NewOutboundController(testWriteCoordinator(t, writer), runtime, &recordingSourceActivation{})
			if err != nil {
				t.Fatal(err)
			}
			controller.Returns = boundary
			command := transport.ControlCommand{Kind: transport.ControlLeaseRevoke, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"}
			if direction == "stale" {
				command.Epoch = "wrong-epoch"
			}
			err = controller.RevokePeer(ctx, command)
			if direction == "stale" {
				if err == nil {
					t.Fatal("stale revoke succeeded")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			want := 0
			if direction == "inbound" {
				want = 1
			}
			if len(boundary.disarmed) != want {
				t.Fatalf("disarmed=%+v, want %d", boundary.disarmed, want)
			}
			if want == 1 && boundary.disarmed[0] != (control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}) {
				t.Fatal("wrong inbound owner retired", boundary.disarmed)
			}
			if len(writer.frames) != 0 {
				t.Fatal("received revoke was echoed")
			}
		})
	}
}
