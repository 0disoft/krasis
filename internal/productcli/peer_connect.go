package productcli

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

// Native I/O is supplied by client. Optional clock/ticker/abort dependencies keep
// deadline admission testable without changing the existing command callers.
type peerConnectOptions struct {
	now    func() time.Time
	ticker func(time.Duration) (<-chan time.Time, func())
	signal context.Context
}

// Submit once, then observe only this exact receipt. Unknown delivery never retries START.
func connectPeerApp(ctx context.Context, client apiClient, args []string, options ...peerConnectOptions) (any, int, error) {
	opt := peerConnectOptions{now: time.Now, ticker: func(interval time.Duration) (<-chan time.Time, func()) {
		tick := time.NewTicker(interval)
		return tick.C, tick.Stop
	}}
	if len(options) > 0 {
		if options[0].now != nil {
			opt.now = options[0].now
		}
		if options[0].ticker != nil {
			opt.ticker = options[0].ticker
		}
		opt.signal = options[0].signal
	}
	ctx, cancel := context.WithTimeout(ctx, 70*time.Second)
	defer cancel()
	if opt.signal != nil {
		observed, abort := context.WithCancel(ctx)
		stop := context.AfterFunc(opt.signal, abort)
		defer func() { stop(); abort() }()
		ctx = observed
	}
	result := appOpenResult{SchemaVersion: "krasis.cli.peer-connect/v1", PeerAlias: args[1]}
	statusOnly := args[0] == "connect-status"
	if statusOnly {
		result.ID = args[2]
	}
	stopped := func() bool {
		if ctx.Err() != nil || opt.signal != nil && opt.signal.Err() != nil {
			return true
		}
		deadline, ok := ctx.Deadline()
		return ok && !opt.now().Before(deadline)
	}
	interrupted := func() (any, int, error) {
		result.Code = "connection_unconfirmed"
		if result.ID == "" && (ctx.Err() == context.Canceled || opt.signal != nil && opt.signal.Err() != nil) {
			result.Code = "canceled"
		}
		return result, ExitUnsafeState, nil
	}
	if stopped() {
		return interrupted()
	}
	peer, code, err := resolvePeer(ctx, client, args[1])
	if stopped() {
		return interrupted()
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
	if !statusOnly {
		if stopped() {
			return interrupted()
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
	if stopped() {
		return interrupted()
	}
	ticks, stopTicks := opt.ticker(time.Second)
	defer stopTicks()
	for {
		if stopped() {
			return interrupted()
		}
		status, err := api.OpenApp(ctx, localapi.AppOpenRequest{Action: "CONNECT_STATUS", Peer: peer, ID: result.ID})
		if stopped() || err != nil || status.ID != result.ID {
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
		case <-ticks:
		}
	}
}
