package productcli

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

const updateWaitSchema = "krasis.cli.update-wait/v1"

type updateWaitResult struct {
	SchemaVersion string `json:"schemaVersion"`
	PeerAlias     string `json:"peerAlias"`
	ID            string `json:"id"`
	Code          string `json:"code"`
	TransferState string `json:"transferState,omitempty"`
	Generation    int    `json:"generation,omitempty"`
	Revision      string `json:"revision,omitempty"`
	Bytes         int64  `json:"bytes,omitempty"`
	Total         int64  `json:"total,omitempty"`
}

func parseUpdateWait(args []string) (time.Duration, error) {
	return parseWaitOptions(args, "update")
}

func dispatchUpdateWait(ctx context.Context, client apiClient, args []string) (any, int, error) {
	timeout, err := parseUpdateWait(args)
	if err != nil {
		return nil, ExitUsage, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	peer, code, err := resolvePeer(ctx, client, args[1])
	if err != nil {
		if ctx.Err() != nil {
			return interruptedUpdateWait(ctx, updateWaitResult{SchemaVersion: updateWaitSchema, PeerAlias: args[1], ID: args[2], Code: "outcome_unknown"})
		}
		return nil, ExitInternal, err
	}
	if code != "" {
		return updateWaitResult{SchemaVersion: updateWaitSchema, PeerAlias: args[1], ID: args[2], Code: code}, ExitUnsafeState, nil
	}
	return waitUpdate(ctx, client, args[1], peer, args[2], 500*time.Millisecond)
}

// Observe one sender receipt. Never queue/retry SEND or treat delivery as application.
func waitUpdate(ctx context.Context, client apiClient, alias, peer, id string, interval time.Duration) (updateWaitResult, int, error) {
	v := updateWaitResult{SchemaVersion: updateWaitSchema, PeerAlias: alias, ID: id, Code: "outcome_unknown"}
	before, err := client.Runtime(ctx)
	if err != nil {
		return interruptedUpdateWait(ctx, v)
	}
	for {
		if ctx.Err() != nil {
			break
		}
		result, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
		if err != nil {
			break
		}
		after, err := client.Runtime(ctx)
		if err != nil {
			break
		}
		if after.Instance != before.Instance {
			v.Code = "agent_changed"
			return v, ExitUnsafeState, nil
		}
		if result.Code != "ok" {
			v.Code = "status_unavailable"
			return v, ExitUnsafeState, nil
		}
		s := result.Status
		if s.ID != id || s.Peer != peer {
			v.Code = "transfer_replaced"
			return v, ExitUnsafeState, nil
		}
		v.TransferState, v.Generation, v.Revision, v.Bytes, v.Total = s.State, s.Generation, s.Revision, s.Bytes, s.Total
		switch s.State {
		case "UPDATED":
			if s.Generation < 1 || !validHex(s.Revision, 40) || s.PeerGeneration != s.Generation || s.PeerRevision != s.Revision {
				return v, ExitUnsafeState, nil
			}
			v.Code = "updated"
			return v, ExitOK, nil
		case "QUEUED", "SENDING", "DELIVERED", "TRANSFER_UNKNOWN", "RESTART_UNCONFIRMED":
			// Await the authenticated observation of the exact target build.
		default:
			v.Code = "update_failed"
			return v, ExitUnsafeState, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return interruptedUpdateWait(ctx, v)
}

func interruptedUpdateWait(ctx context.Context, v updateWaitResult) (updateWaitResult, int, error) {
	if ctx.Err() == context.DeadlineExceeded {
		v.Code = "timeout"
	} else if ctx.Err() != nil {
		v.Code = "canceled"
	}
	return v, ExitUnsafeState, nil
}
