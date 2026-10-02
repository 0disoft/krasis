package productcli

import (
	"context"
	"time"
)

func dispatchDiagnosticWait(ctx context.Context, client apiClient, args []string) (any, int, error) {
	timeout, err := parseWaitOptions(args, "diagnostics")
	if err != nil {
		return nil, ExitUsage, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	v := diagnosticCollectionResult{SchemaVersion: diagnosticCollectionSchema, PeerAlias: args[1], ID: args[2], Code: "outcome_unknown"}
	peer, code, err := resolvePeer(ctx, client, args[1])
	if err != nil {
		if ctx.Err() != nil {
			return interruptedDiagnosticWait(ctx, v)
		}
		return nil, ExitInternal, err
	}
	if code != "" {
		v.Code = code
		return v, ExitUnsafeState, nil
	}
	return waitDiagnostic(ctx, client, v, peer, time.Second)
}

// Observe the existing request only. Never start another export after a lost reply.
func waitDiagnostic(ctx context.Context, client apiClient, v diagnosticCollectionResult, peer string, interval time.Duration) (diagnosticCollectionResult, int, error) {
	before, err := client.Runtime(ctx)
	if err != nil {
		return interruptedDiagnosticWait(ctx, v)
	}
	for {
		if ctx.Err() != nil {
			return interruptedDiagnosticWait(ctx, v)
		}
		result, exit, _ := collectionStatus(ctx, client, v, peer, v.ID)
		v = result
		after, err := client.Runtime(ctx)
		if err != nil || ctx.Err() != nil {
			v.Code, v.State, v.LocalPath, v.PeerPath = "outcome_unknown", "", "", ""
			return interruptedDiagnosticWait(ctx, v)
		}
		if after.Instance != before.Instance {
			v.Code, v.State, v.LocalPath, v.PeerPath = "agent_changed", "", "", ""
			return v, ExitUnsafeState, nil
		}
		if v.Code != "pending" {
			return v, exit, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

func interruptedDiagnosticWait(ctx context.Context, v diagnosticCollectionResult) (diagnosticCollectionResult, int, error) {
	if ctx.Err() == context.DeadlineExceeded {
		v.Code = "timeout"
	} else if ctx.Err() != nil {
		v.Code = "canceled"
	}
	return v, ExitUnsafeState, nil
}
