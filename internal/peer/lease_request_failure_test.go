package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func TestLeaseRequestFailurePreservesEntryAndRollbackCauses(t *testing.T) {
	for _, stage := range []string{"authorize", "entry", "grant"} {
		t.Run(stage, func(t *testing.T) {
			runtime := newLocalRuntime(t)
			writer := &recordingFrameWriter{}
			original := errors.New("private-native-path")
			cleanup := errors.New("private-rollback")
			entry := &recordingCursorEntry{rollbackErr: cleanup}
			authorizer := staticLeaseAuthorizer{accepted: true, leaseID: "lease-1"}
			switch stage {
			case "authorize":
				authorizer.err = original
			case "entry":
				entry.err = original
			case "grant":
				writer.err = original
			}
			responder := &LeaseResponder{Coordinator: testWriteCoordinator(t, writer), Runtime: runtime, Authorizer: authorizer, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}
			err := responder.HandleLeaseRequest(context.Background(), transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "s", Epoch: "e", Edge: control.ScreenEdgeRight}, time.Now())
			var failure *LeaseRequestFailure
			if !errors.As(err, &failure) || failure.Stage != stage || !errors.Is(err, original) {
				t.Fatal("failed request phase/cause lost", err)
			}
			if stage != "authorize" && (!errors.Is(err, cleanup) || entry.rollbacks != 1 || len(entry.disarmed) != 1) {
				t.Fatal("rollback failure lost or cleanup changed", err, entry.rollbacks)
			}
			if entry.commits != 0 || runtime.Snapshot().Control.State == control.StateInboundActive {
				t.Fatal("failed request granted input")
			}
		})
	}
}
