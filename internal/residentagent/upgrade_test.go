package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"testing"
)

func TestUpgradeStopRejectsActiveSessionAndRetiresIdleAgent(t *testing.T) {
	ops := testOperations(t, nil)
	ops.sessionDone = make(chan struct{})
	result, err := ops.StopForUpgrade(context.Background())
	if err != nil || result.Code != "active_session_requires_disconnect" || ops.Context().Err() != nil {
		t.Fatal("active agent stopped")
	}
	ops.sessionDone = nil
	ops.pairing = upgradePairing{}
	result, err = ops.StopForUpgrade(context.Background())
	if err != nil || result.Code != "pairing_already_active" || ops.Context().Err() != nil {
		t.Fatal("pairing agent stopped")
	}
	ops.pairing = nil
	result, err = ops.StopForUpgrade(context.Background())
	if err != nil || result.Code != "upgrade_stop_requested" || ops.Context().Err() == nil {
		t.Fatal("idle stop failed")
	}
	result, err = ops.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
	if err != nil || result.Code != "operation_cancelled" {
		t.Fatal("new session accepted after retirement")
	}
}

type upgradePairing struct{}

func (upgradePairing) Active() bool { return true }
func (upgradePairing) Handle(context.Context, localapi.OperationRequest) (localapi.OperationResult, error) {
	return localapi.OperationResult{}, nil
}
