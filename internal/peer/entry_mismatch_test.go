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

type mismatchedCursorEntry struct {
	retiringCursorEntry
	outcome application.CursorRollbackOutcome
}

func (entry *mismatchedCursorEntry) RollbackEntry(ctx context.Context) (application.CursorRollbackOutcome, error) {
	_, err := entry.retiringCursorEntry.RollbackEntry(ctx)
	return entry.outcome, err
}

func TestEntryMismatchRejectsOnlyAfterKnownCleanupAndAllowsNextLease(t *testing.T) {
	for _, outcome := range []application.CursorRollbackOutcome{application.CursorRollbackRestored, application.CursorRollbackNotRestored} {
		t.Run(string(outcome), func(t *testing.T) {
			ctx := context.Background()
			entry := &mismatchedCursorEntry{outcome: outcome}
			entry.err = &application.CursorTransactionFailure{Operation: "entry", Stage: "mismatch", Cause: application.ErrCursorEntryMismatch}
			runtime, writer := newLocalRuntime(t), &recordingFrameWriter{}
			responder := &LeaseResponder{Coordinator: testWriteCoordinator(t, writer), Runtime: runtime,
				Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true}, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}
			command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session-1", Epoch: "epoch-1", Edge: control.ScreenEdgeRight}
			if err := responder.HandleLeaseRequest(ctx, command, time.Now()); err != nil {
				t.Fatal("known rollback terminated connection", err)
			}
			reject := decodeWrittenControl(t, writer)
			if len(writer.frames) != 1 || reject.Kind != transport.ControlLeaseReject || reject.SessionID != command.SessionID || reject.Epoch != command.Epoch || reject.LeaseID != "" {
				t.Fatal("rejection lost identity or granted input", reject)
			}
			if entry.rollbacks != 1 || entry.commits != 0 || entry.retiredDuringRollback || !entry.retired || entry.cleanupErr != nil || !entry.cleanupBounded || responder.pending != nil {
				t.Fatal("rejection skipped cleanup or retired backend before rollback")
			}
			if s := runtime.Snapshot(); s.Control.State != control.StateLocal || s.HasStream {
				t.Fatal("rejected entry retained inbound state", s)
			}
			// The source handles the same rejection while still unsuppressed.
			source, sourceRuntime := &recordingSourceActivation{}, newLocalRuntime(t)
			controller, err := NewOutboundController(testWriteCoordinator(t, &recordingFrameWriter{}), sourceRuntime, source)
			if err != nil {
				t.Fatal(err)
			}
			if err := controller.Begin(ctx, command.SessionID, command.Epoch, command.Edge); err != nil {
				t.Fatal(err)
			}
			if err := controller.RejectPeer(ctx, reject); err != nil || source.active || source.activations != 0 || sourceRuntime.Snapshot().Control.State != control.StateLocal {
				t.Fatal("source rejection failed safe local recovery", err)
			}
			entry.err = nil
			command.SessionID, command.Epoch = "session-2", "epoch-2"
			if err := responder.HandleLeaseRequest(ctx, command, time.Now()); err != nil || responder.pending == nil || len(writer.frames) != 2 {
				t.Fatal("next request required a new connection", err)
			}
			grant, err := transport.DecodeControlPayload(writer.frames[1].Payload)
			if err != nil || grant.Kind != transport.ControlLeaseGrant || grant.SessionID != command.SessionID || grant.Epoch != command.Epoch {
				t.Fatal("next lease did not get an exact grant", grant, err)
			}
		})
	}
}

func TestEntryMismatchDoesNotHideUncertaintyCleanupCancellationOrWriteFailure(t *testing.T) {
	for _, mode := range []string{"unmarked_mismatch", "unknown_write", "rollback_failed", "unknown_rollback", "canceled", "reject_write_failed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entry := &mismatchedCursorEntry{outcome: application.CursorRollbackRestored}
			entry.err = &application.CursorTransactionFailure{Operation: "entry", Stage: "mismatch", Cause: application.ErrCursorEntryMismatch}
			cause := errors.New("test failure")
			writer := &recordingFrameWriter{}
			switch mode {
			case "unmarked_mismatch":
				entry.err = &application.CursorTransactionFailure{Operation: "entry", Stage: "mismatch", Cause: cause}
			case "unknown_write":
				entry.err = &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: cause}
			case "rollback_failed":
				entry.rollbackErr = cause
			case "unknown_rollback":
				entry.outcome = ""
			case "canceled":
				entry.cancelOnPrepare = cancel
			case "reject_write_failed":
				writer.err = cause
			}
			responder := &LeaseResponder{Coordinator: testWriteCoordinator(t, writer), Runtime: newLocalRuntime(t),
				Authorizer: staticLeaseAuthorizer{leaseID: "l", accepted: true}, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}
			command := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "s", Epoch: "e", Edge: control.ScreenEdgeLeft}
			err := responder.HandleLeaseRequest(ctx, command, time.Now())
			if err == nil || entry.rollbacks != 1 || entry.commits != 0 || entry.retiredDuringRollback || !entry.cleanupBounded || entry.cleanupErr != nil || responder.pending != nil {
				t.Fatal("failure was hidden or cleanup was canceled", err)
			}
			if mode == "reject_write_failed" {
				var failure *LeaseRequestFailure
				if !errors.Is(err, cause) || !errors.As(err, &failure) || failure.Stage != "entry-reject" || len(writer.frames) != 1 {
					t.Fatal("reject write failure hidden", err)
				}
			} else if len(writer.frames) != 0 {
				t.Fatal("uncertain entry sent a reply")
			}
			if mode == "rollback_failed" && (!errors.Is(err, cause) || !errors.Is(err, application.ErrCursorEntryMismatch)) {
				t.Fatal("cleanup or original cause lost", err)
			}
		})
	}
}
