package productcli

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type appOpenResult struct {
	SchemaVersion string `json:"schemaVersion"`
	Code          string `json:"code"`
	PeerAlias     string `json:"peerAlias"`
	ID            string `json:"id,omitempty"`
}

func openPeerApp(ctx context.Context, client apiClient, args []string) (any, int, error) {
	result := appOpenResult{SchemaVersion: "krasis.cli.app-open/v1", PeerAlias: args[1]}
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
	if args[0] == "open-status" {
		result.ID = args[2]
		status, err := api.OpenApp(ctx, localapi.AppOpenRequest{Action: "STATUS", Peer: peer, ID: result.ID})
		return appOpenOutcome(result, status, err)
	}
	started, err := api.OpenApp(ctx, localapi.AppOpenRequest{Action: "START", Peer: peer})
	if err != nil {
		result.Code = "outcome_unknown"
		return result, ExitUnsafeState, nil
	}
	if started.Code != "queued" || !validHex(started.ID, 32) {
		return appOpenOutcome(result, started, nil)
	}
	result.ID, result.Code = started.ID, "pending"
	await, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-await.Done():
			return result, ExitUnsafeState, nil
		case <-ticker.C:
			status, err := api.OpenApp(await, localapi.AppOpenRequest{Action: "STATUS", Peer: peer, ID: result.ID})
			if err != nil {
				result.Code = "outcome_unknown"
				return result, ExitUnsafeState, nil
			}
			if status.Code != "pending" {
				return appOpenOutcome(result, status, nil)
			}
		}
	}
}

func appOpenOutcome(result appOpenResult, status localapi.AppOpenResult, err error) (any, int, error) {
	result.Code = status.Code
	if err != nil {
		result.Code = "outcome_unknown"
	}
	if result.Code == "launch_requested" || result.Code == "window_ready" {
		return result, ExitOK, nil
	}
	if result.Code == "unsupported" || result.Code == "rejected" {
		return result, ExitPolicyDenied, nil
	}
	return result, ExitUnsafeState, nil
}
