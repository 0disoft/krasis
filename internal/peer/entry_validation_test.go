package peer

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"testing"
	"time"
)

type validatingEntry struct {
	recordingCursorEntry
	err error
}

func (e *validatingEntry) ValidateEntry(context.Context) error { return e.err }

func TestPreparedEntryValidationFailureRollsBackBeforeCommit(t *testing.T) {
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	entry := &validatingEntry{}
	responder := &LeaseResponder{Coordinator: testWriteCoordinator(t, writer), Runtime: runtime, Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true}, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}
	request := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
	if err := responder.HandleLeaseRequest(context.Background(), request, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	entry.err = errors.New("geometry changed after prepare")
	commit := transport.ControlCommand{Kind: transport.ControlLeaseCommit, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"}
	if err := responder.HandleLeaseCommit(context.Background(), commit, time.Unix(101, 0)); !errors.Is(err, entry.err) {
		t.Fatalf("commit accepted: %v", err)
	}
	if entry.commits != 0 || entry.rollbacks != 1 || len(entry.disarmed) != 1 || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatal("pending entry was not safely rolled back")
	}
}
