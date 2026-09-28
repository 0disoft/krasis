package productcli

import (
	"context"

	"github.com/0disoft/krasis/internal/localapi"
)

const clipboardSchema = "krasis.cli.clipboard/v1"

type clipboardState struct {
	SchemaVersion   string `json:"schemaVersion"`
	PeerAlias       string `json:"peerAlias"`
	Code            string `json:"code"`
	Enabled         bool   `json:"enabled"`
	OtherPeerActive bool   `json:"otherPeerActive"`
}

func dispatchClipboard(ctx context.Context, client apiClient, args []string) (any, int, error) {
	if len(args) != 2 {
		return nil, ExitUsage, usageError("clipboard requires status|on|off ALIAS")
	}
	peer, code, err := resolvePeer(ctx, client, args[1])
	if err != nil {
		return nil, ExitInternal, err
	}
	v := clipboardState{SchemaVersion: clipboardSchema, PeerAlias: args[1], Code: code}
	if code != "" {
		return v, ExitUnsafeState, nil
	}
	status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	if err != nil {
		return nil, ExitInternal, err
	}
	if status.Code != "ok" {
		v.Code = "status_unavailable"
		return v, ExitUnsafeState, nil
	}
	v.Enabled = status.Status.ClipboardPeer == peer
	v.OtherPeerActive = status.Status.ClipboardPeer != "" && !v.Enabled
	if args[0] == "status" {
		v.Code = "ok"
		return v, ExitOK, nil
	}
	if v.OtherPeerActive {
		v.Code = "other_peer_active"
		return v, ExitUnsafeState, nil
	}
	want := args[0] == "on"
	if v.Enabled == want {
		v.Code = "ok"
		return v, ExitOK, nil
	}
	action := "DISABLE"
	if want {
		action = "ENABLE"
	}
	result, err := client.Clipboard(ctx, localapi.ClipboardRequest{Action: action, Peer: peer})
	if err != nil {
		// The agent may have applied the request before the response was lost.
		return nil, ExitInternal, err
	}
	if result.Code != "ok" {
		v.Code = result.Code
		return v, ExitUnsafeState, nil
	}
	observed, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	if err != nil || observed.Code != "ok" || (observed.Status.ClipboardPeer == peer) != want {
		v.Code = "outcome_unknown"
		return v, ExitUnsafeState, nil
	}
	v.Enabled, v.Code = want, "ok"
	return v, ExitOK, nil
}
