package productcli

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

// Submit once, then observe only this exact receipt. Unknown delivery never retries START.
func connectPeerApp(ctx context.Context, client apiClient, args []string) (any, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	result := appOpenResult{SchemaVersion: "krasis.cli.peer-connect/v1", PeerAlias: args[1]}
	peer, code, err := resolvePeer(ctx, client, args[1])
	if err != nil {
		return nil, ExitInternal, err
	}
	if code != "" {
		result.Code = code
		return result, ExitUnsafeState, nil
	}
	api, ok := client.(interface {
		OpenApp(context.Context, localapi.AppOpenRequest) (localapi.AppOpenResult, error)
	})
	if !ok {
		result.Code = "unsupported"
		return result, ExitPolicyDenied, nil
	}
	statusOnly := args[0] == "connect-status"
	if statusOnly {
		result.ID = args[2]
	} else {
		if ctx.Err() != nil {
			result.Code = "canceled"
			return result, ExitUnsafeState, nil
		}
		start, err := api.OpenApp(ctx, localapi.AppOpenRequest{Action: "CONNECT", Peer: peer})
		if err != nil {
			result.Code = "outcome_unknown"
			return result, ExitUnsafeState, nil
		}
		if start.Code != "queued" || !validHex(start.ID, 32) {
			result.Code = "outcome_unknown"
			if start.Code == "busy" || start.Code == "rejected" || start.Code == "unsupported" {
				result.Code = start.Code
			}
			return result, ExitUnsafeState, nil
		}
		result.ID = start.ID
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			result.Code = "connection_unconfirmed"
			return result, ExitUnsafeState, nil
		}
		status, err := api.OpenApp(ctx, localapi.AppOpenRequest{Action: "CONNECT_STATUS", Peer: peer, ID: result.ID})
		if err != nil || ctx.Err() != nil || status.ID != result.ID {
			result.Code = "outcome_unknown"
			return result, ExitUnsafeState, nil
		}
		result.Code = status.Code
		switch status.Code {
		case "connected":
			return result, ExitOK, nil
		case "pending", "accepted":
		case "busy", "rejected", "unsupported", "connection_unconfirmed", "unknown_request":
			return result, ExitUnsafeState, nil
		default:
			result.Code = "outcome_unknown"
			return result, ExitUnsafeState, nil
		}
		if statusOnly {
			return result, ExitUnsafeState, nil
		}
		select {
		case <-ctx.Done():
			result.Code = "connection_unconfirmed"
			return result, ExitUnsafeState, nil
		case <-tick.C:
		}
	}
}
