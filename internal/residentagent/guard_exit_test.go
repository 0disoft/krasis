package residentagent

import (
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestUnconfirmedGuardExitFencesManagedReplacement(t *testing.T) {
	ops := testOperations(t, nil)
	done, guardDone := make(chan struct{}), make(chan struct{})
	outcome := &sessionOutcome{connected: true}
	ops.sessionDone, ops.sessionOutcome = done, outcome
	ops.finishSession(done, outcome, &crashguard.Failure{Operation: "close", Cause: &crashguard.ExitPending{Done: guardDone, Cause: crashguard.ErrExitTimeout}}, false, false)
	if result, err := ops.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); err != nil || result.Code != "session_failed" {
		t.Fatal(result, err)
	}
	if result, err := ops.SafeStop(context.Background()); err != nil || result.Code != "guard_exit_pending" || ops.Context().Err() != nil {
		t.Fatal(result, err)
	}
	if result, err := ops.StopForUpgrade(context.Background(), 195, ""); err != nil || result.Code != "guard_exit_pending" || ops.Context().Err() != nil {
		t.Fatal(result, err)
	}
	close(guardDone)
	if result, err := ops.StopForUpgrade(context.Background(), 195, ""); err != nil || result.Code != "upgrade_stop_requested" || ops.Context().Err() == nil {
		t.Fatal(result, err)
	}
}
