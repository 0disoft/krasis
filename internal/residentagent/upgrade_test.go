package residentagent

import (
	"context"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestUpgradeStopRejectsActiveSessionAndRetiresIdleAgent(t *testing.T) {
	ops := testOperations(t, nil)
	revision := strings.Repeat("a", 40)
	ops.updateApplying = true
	ops.updateTargetGeneration = 83
	ops.updateTargetRevision = revision
	result, err := ops.StopForUpgrade(context.Background(), 82, revision)
	if err != nil || result.Code != "update_apply_in_progress" || ops.Context().Err() != nil {
		t.Fatal("update apply was interrupted")
	}
	result, err = ops.StopForUpgrade(context.Background(), 83, strings.Repeat("b", 40))
	if err != nil || result.Code != "update_apply_in_progress" || ops.Context().Err() != nil {
		t.Fatal("different update build interrupted apply")
	}
	ops.updateApplying = false
	ops.sessionDone = make(chan struct{})
	result, err = ops.StopForUpgrade(context.Background(), 83, revision)
	if err != nil || result.Code != "active_session_requires_disconnect" || ops.Context().Err() != nil {
		t.Fatal("active agent stopped")
	}
	ops.sessionDone = nil
	ops.pairing = upgradePairing{}
	result, err = ops.StopForUpgrade(context.Background(), 83, revision)
	if err != nil || result.Code != "pairing_already_active" || ops.Context().Err() != nil {
		t.Fatal("pairing agent stopped")
	}
	ops.pairing = nil
	result, err = ops.StopForUpgrade(context.Background(), 83, revision)
	if err != nil || result.Code != "upgrade_stop_requested" || ops.Context().Err() == nil {
		t.Fatal("idle stop failed")
	}
	result, err = ops.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Code != "operation_cancelled" {
		t.Fatal("new session accepted after retirement")
	}
}

func TestUpgradeStopAcceptsExactClaimedUpdateBuild(t *testing.T) {
	ops := testOperations(t, nil)
	revision := strings.Repeat("a", 40)
	ops.updateApplying = true
	ops.updateTargetGeneration = 83
	ops.updateTargetRevision = revision
	result, err := ops.StopForUpgrade(context.Background(), 83, revision)
	if err != nil || result.Code != "upgrade_stop_requested" || ops.Context().Err() == nil {
		t.Fatal("verified update could not replace resident agent")
	}
}

type upgradePairing struct{}

func (upgradePairing) Active() bool { return true }
func (upgradePairing) Handle(context.Context, localapi.OperationRequest) (localapi.OperationResult, error) {
	return localapi.OperationResult{}, nil
}
