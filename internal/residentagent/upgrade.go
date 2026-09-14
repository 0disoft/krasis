package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
)

// The idle check and root cancellation share Connect/Pair/Credential's lock.
func (operations *Operations) StopForUpgrade(ctx context.Context) (localapi.OperationResult, error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if ctx.Err() != nil || operations.root.Err() != nil {
		return rejected("operation_cancelled"), nil
	}
	if operations.sessionDone != nil {
		return rejected("active_session_requires_disconnect"), nil
	}
	if pairing, ok := operations.pairing.(interface{ Active() bool }); ok && pairing.Active() {
		return rejected("pairing_already_active"), nil
	}
	operations.log.Event("upgrade_stop_requested")
	operations.stopAgent(errors.New("idle agent upgrade requested"))
	return completed("upgrade_stop_requested"), nil
}
