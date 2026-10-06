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
	ctx, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	result := appOpenResult{SchemaVersion: "krasis.cli.app-open/v1", PeerAlias: args[1]}
	peer, code, err := resolvePeer(ctx, client, args[1])
	if ctx.Err() != nil {
		return interruptedAppOpen(ctx, result)
	}
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
		if ctx.Err() != nil {
			return interruptedAppOpen(ctx, result)
		}
		return appOpenOutcome(result, status, err)
	}
	started, err := api.OpenApp(ctx, localapi.AppOpenRequest{Action: "START", Peer: peer})
	if err != nil {
		result.Code = "outcome_unknown"
		if ctx.Err() == context.Canceled {
			result.Code = "canceled"
		}
		return result, ExitUnsafeState, nil
	}
	if started.Code != "queued" {
		switch started.Code {
		case "unsupported", "rejected", "busy":
			return appOpenOutcome(result, started, nil)
		default:
			result.Code = "outcome_unknown"
			return result, ExitUnsafeState, nil
		}
	}
	if !validHex(started.ID, 32) {
		result.Code = "outcome_unknown"
		return result, ExitUnsafeState, nil
	}
	result.ID, result.Code = started.ID, "pending"
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return interruptedAppOpen(ctx, result)
		case <-ticker.C:
			if ctx.Err() != nil {
				return interruptedAppOpen(ctx, result)
			}
			status, err := api.OpenApp(ctx, localapi.AppOpenRequest{Action: "STATUS", Peer: peer, ID: result.ID})
			if ctx.Err() != nil {
				return interruptedAppOpen(ctx, result)
			}
			next, exit, _ := appOpenOutcome(result, status, err)
			switch next.Code {
			case "pending", "launch_accepted", "launch_requested":
				result = next
			default:
				return next, exit, nil
			}
		}
	}
}

func appOpenOutcome(result appOpenResult, status localapi.AppOpenResult, err error) (appOpenResult, int, error) {
	result.Code = status.Code
	if err != nil || status.ID != result.ID || (status.Code == "window_ready" && result.ID == "") {
		result.Code = "outcome_unknown"
		return result, ExitUnsafeState, nil
	}
	switch result.Code {
	case "window_ready":
		return result, ExitOK, nil
	case "unsupported", "rejected":
		return result, ExitPolicyDenied, nil
	case "pending", "launch_accepted", "launch_requested", "window_unconfirmed", "launch_failed",
		"launch_record_missing", "launch_record_invalid", "expired", "peer_unresponsive", "unknown_request", "busy":
	default:
		result.Code = "outcome_unknown"
	}
	return result, ExitUnsafeState, nil
}

func interruptedAppOpen(ctx context.Context, result appOpenResult) (appOpenResult, int, error) {
	if ctx.Err() == context.Canceled {
		result.Code = "canceled"
	} else if result.Code == "launch_accepted" || result.Code == "launch_requested" {
		result.Code = "window_unconfirmed"
	} else if result.ID != "" {
		result.Code = "outcome_unknown"
	} else {
		result.Code = "timeout"
	}
	return result, ExitUnsafeState, nil
}
