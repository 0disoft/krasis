package productpanel

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type AppOpenSnapshot struct {
	Schema string `json:"schema"`
	Peer   string `json:"peer"`
	Code   string `json:"code"`
	ID     string `json:"id"`
}

// Only the trusted receiver's remembered UI can be opened. No path, command or
// URL comes from the renderer. START is never retried after an uncertain reply.
func (b *UpdateSettingsBridge) AppOpen(peer, id string) AppOpenSnapshot {
	v := AppOpenSnapshot{Schema: "krasis.app-open/v1", Peer: peer, Code: "unavailable", ID: id}
	valid := func(value string, size int) bool {
		decoded, err := hex.DecodeString(value)
		return err == nil && len(decoded) == size
	}
	if !valid(peer, 32) || (id != "" && !valid(id, 16)) {
		v.Peer, v.ID, v.Code = "", "", "invalid_peer"
		return v
	}
	if b == nil || b.ctx == nil || b.open == nil {
		return v
	}
	if !b.mu.TryLock() {
		v.Code = "query_busy"
		return v
	}
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
	defer cancel()
	c, err := b.open()
	if err != nil || c == nil {
		return v
	}
	trust, err := c.Trust(ctx)
	if err != nil {
		return v
	}
	trusted := false
	for _, p := range trust.Peers {
		if p.Fingerprint == peer && !p.Revoked {
			trusted = true
			break
		}
	}
	if !trusted {
		v.Code = "stale_peer"
		return v
	}
	api, ok := c.(interface {
		OpenApp(context.Context, localapi.AppOpenRequest) (localapi.AppOpenResult, error)
	})
	if !ok {
		v.Code = "unsupported"
		return v
	}
	action := "START"
	if id != "" {
		action = "STATUS"
	}
	r, err := api.OpenApp(ctx, localapi.AppOpenRequest{Action: action, Peer: peer, ID: id})
	if err != nil {
		if action == "START" {
			v.Code = "outcome_unknown"
		}
		return v
	}
	if action == "START" && r.Code == "queued" {
		if !valid(r.ID, 16) {
			v.Code = "outcome_unknown"
			return v
		}
		v.Code, v.ID = "pending", r.ID
		return v
	}
	if action == "STATUS" && r.ID != id {
		return v
	}
	switch r.Code {
	case "pending", "launch_requested", "window_ready", "window_unconfirmed", "launch_failed", "launch_record_missing", "launch_record_invalid", "expired", "peer_unresponsive", "unknown_request":
		if action == "STATUS" {
			v.Code = r.Code
		}
	case "rejected", "unsupported", "busy":
		v.Code = r.Code
	}
	return v
}
