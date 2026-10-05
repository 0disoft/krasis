package peer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

// Installed input retires the cursor backend when DisarmReturn is called.
// A recording-only return detector cannot expose premature backend retirement.
type retiringCursorEntry struct {
	recordingCursorEntry
	retired, retiredDuringRollback bool
	cancelOnPrepare                context.CancelFunc
	cleanupErr                     error
	cleanupBounded                 bool
}

func (entry *retiringCursorEntry) PrepareEntry(ctx context.Context, edge control.ScreenEdge) error {
	if entry.cancelOnPrepare != nil {
		entry.cancelOnPrepare()
	}
	return entry.recordingCursorEntry.PrepareEntry(ctx, edge)
}

func (entry *retiringCursorEntry) RollbackEntry(ctx context.Context) (application.CursorRollbackOutcome, error) {
	entry.cleanupErr = ctx.Err()
	_, entry.cleanupBounded = ctx.Deadline()
	entry.retiredDuringRollback = entry.retired
	if entry.retired {
		return "", errors.New("cursor backend already retired")
	}
	return entry.recordingCursorEntry.RollbackEntry(ctx)
}

func (entry *retiringCursorEntry) DisarmReturn(intent control.ReturnIntent) {
	entry.retired = true
	entry.recordingCursorEntry.DisarmReturn(intent)
}

func TestCursorRecoveryPrecedesBackendRetirement(t *testing.T) {
	for _, path := range []string{"entry_failure", "grant_write_failure", "peer_abort", "pending_abort"} {
		for _, recoveryFails := range []bool{false, true} {
			t.Run(path+map[bool]string{false: "/recovered", true: "/recovery_failed"}[recoveryFails], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				runtime := newLocalRuntime(t)
				entry := &retiringCursorEntry{}
				cause := errors.New("initial failure")
				recoveryErr := errors.New("cursor recovery failed")
				if recoveryFails {
					entry.rollbackErr = recoveryErr
				}
				writer := &recordingFrameWriter{}
				if path == "entry_failure" {
					entry.err, entry.cancelOnPrepare = cause, cancel
				}
				if path == "grant_write_failure" {
					writer.err = cause
				}
				responder := &LeaseResponder{
					Coordinator: testWriteCoordinator(t, writer), Runtime: runtime,
					Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
					Peer:       testAuthenticatedPeer(), Entry: entry, Returns: entry,
				}
				request := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
				err := responder.HandleLeaseRequest(ctx, request, time.Now())
				if path == "peer_abort" || path == "pending_abort" {
					if err != nil {
						t.Fatal(err)
					}
					if path == "peer_abort" {
						err = responder.HandleLeaseAbort(ctx, transport.ControlCommand{Kind: transport.ControlLeaseAbort, SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"})
					} else {
						err = responder.AbortPending(ctx)
					}
				} else if !errors.Is(err, cause) {
					t.Fatal("initial failure lost", err)
				}
				if entry.retiredDuringRollback || !entry.retired || len(entry.disarmed) != 1 || entry.rollbacks != 1 {
					t.Fatalf("recovery/retirement order: %+v, err=%v", entry, err)
				}
				if entry.cleanupErr != nil || !entry.cleanupBounded {
					t.Fatal("cleanup cancelled or unbounded")
				}
				if recoveryFails && !errors.Is(err, recoveryErr) {
					t.Fatal("recovery failure hidden", err)
				}
				if !recoveryFails && (path == "peer_abort" || path == "pending_abort") && err != nil {
					t.Fatal(err)
				}
				if responder.pending != nil || runtime.Snapshot().Control.State != control.StateLocal {
					t.Fatal("pending runtime retained")
				}
			})
		}
	}
}

func TestMismatchedAbortCannotRetirePreparedCursorBackend(t *testing.T) {
	entry := &retiringCursorEntry{}
	responder := &LeaseResponder{
		Coordinator: testWriteCoordinator(t, &recordingFrameWriter{}), Runtime: newLocalRuntime(t),
		Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true},
		Peer:       testAuthenticatedPeer(), Entry: entry, Returns: entry,
	}
	request := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
	if err := responder.HandleLeaseRequest(context.Background(), request, time.Now()); err != nil {
		t.Fatal(err)
	}
	abort := transport.ControlCommand{Kind: transport.ControlLeaseAbort, SessionID: "session-1", LeaseID: "stale-lease", Epoch: "epoch-1"}
	if err := responder.HandleLeaseAbort(context.Background(), abort); err == nil {
		t.Fatal("stale abort accepted")
	}
	if entry.retired || entry.rollbacks != 0 || responder.pending == nil {
		t.Fatal("stale abort changed valid lease")
	}
	abort.LeaseID = "lease-1"
	if err := responder.HandleLeaseAbort(context.Background(), abort); err != nil {
		t.Fatal(err)
	}
}
