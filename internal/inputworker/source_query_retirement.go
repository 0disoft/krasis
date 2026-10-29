package inputworker

import "context"

// All command, watchdog, capture and sender callers have joined before entry;
// no caller may submit a replacement query. Cancellation only fences authority:
// the read-only native owner still retains its exact pipe and proof handles.
// Give those owners one shared cleanup budget before requiring service repair.
// Waiting neither retries the command nor accepts its expired authorization.
func sourceCaptureQueryRetirement(result error, owners ...*Channel) error {
	ctx, cancel := context.WithTimeout(context.Background(), RecoveryTimeout)
	defer cancel()
	for _, owner := range owners {
		if owner == nil {
			continue
		}
		owner.queryMu.Lock()
		queries := [4]*authorityQuery{owner.query, owner.presenceQuery, owner.watchdogQuery, owner.maintenanceQuery}
		owner.queryMu.Unlock()
		for _, query := range queries {
			if query == nil {
				continue
			}
			select {
			case <-query.done:
			case <-ctx.Done():
				return sourceCaptureQueriesRetired(result, owners...)
			}
		}
	}
	return sourceCaptureQueriesRetired(result, owners...)
}
