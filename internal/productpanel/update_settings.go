package productpanel

import (
	"context"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

const UpdateSettingsSchema = "krasis.update-settings/v1"

type UpdateSettingsSnapshot struct {
	Schema           string `json:"schema"`
	Code             string `json:"code"`
	Peer             string `json:"peer"`
	Permission       string `json:"permission"`
	RemotePermission string `json:"remotePermission"`
	PeerState        string `json:"peerState"`
	LocalGeneration  int    `json:"localGeneration"`
	LocalBuild       string `json:"localBuild"`
	PeerGeneration   int    `json:"peerGeneration"`
	PeerBuild        string `json:"peerBuild"`
	Comparison       string `json:"comparison"`
}

type UpdateSettingsClient interface {
	Trust(context.Context) (localapi.Trust, error)
	Runtime(context.Context) (localapi.RuntimeInfo, error)
	Update(context.Context, localapi.UpdateRequest) (localapi.UpdateResult, error)
}

type UpdateSettingsBridge struct {
	ctx  context.Context
	open func() (UpdateSettingsClient, error)
	mu   sync.Mutex
}

func NewUpdateSettingsBridge(ctx context.Context, open func() (UpdateSettingsClient, error)) *UpdateSettingsBridge {
	return &UpdateSettingsBridge{ctx: ctx, open: open}
}
func (b *UpdateSettingsBridge) Status(peer string) UpdateSettingsSnapshot {
	return b.execute(peer, "STATUS")
}
func (b *UpdateSettingsBridge) Permit(peer string, allow bool) UpdateSettingsSnapshot {
	action := "DENY"
	if allow {
		action = "ALLOW"
	}
	return b.execute(peer, action)
}

func updateSettingsFailure(peer, code string) UpdateSettingsSnapshot {
	return UpdateSettingsSnapshot{Schema: UpdateSettingsSchema, Code: code, Peer: peer, Permission: "UNKNOWN", RemotePermission: "UNKNOWN", PeerState: "UNKNOWN", Comparison: "UNKNOWN"}
}
func (b *UpdateSettingsBridge) execute(peer, action string) UpdateSettingsSnapshot {
	v := updateSettingsFailure("", "unavailable")
	if b == nil || b.ctx == nil || b.open == nil {
		return v
	}
	if len(peer) != 64 {
		v.Code = "invalid_peer"
		return v
	}
	if _, err := hex.DecodeString(peer); err != nil {
		v.Code = "invalid_peer"
		return v
	}
	v.Peer = peer
	if !b.mu.TryLock() {
		v.Code = "busy"
		return v
	}
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		v.Code = "cancelled"
		return v
	}
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
	before, runtimeErr := c.Runtime(ctx)
	if runtimeErr == nil {
		setLocalUpdateVersion(&v, before)
	}
	r := localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: action}
	if action != "STATUS" {
		r.Peer = peer
	}
	if ctx.Err() != nil {
		v.Code = "cancelled"
		return v
	}
	result, err := c.Update(ctx, r)
	if err != nil {
		if action != "STATUS" {
			v.Code = "outcome_unknown"
		}
		return v
	}
	expected := "ok"
	if action != "STATUS" {
		expected = "permission_saved"
	}
	if result.Code != expected {
		v.Code = "rejected"
		if result.Code == "unsupported" {
			v.Code = "unsupported"
		}
		return v
	}
	v.Permission = updatePermission(result.Status.Allowed, peer)
	v.Code = expected
	if action != "STATUS" {
		if runtimeErr == nil && before.Instance != "" {
			after, err := c.Runtime(ctx)
			if err != nil || after.Instance != before.Instance {
				return updateSettingsFailure(peer, "agent_changed")
			}
		}
		if (action == "ALLOW") != (v.Permission == "ALLOWED") {
			v.Permission = "UNKNOWN"
			v.Code = "outcome_unknown"
		}
		return v
	}
	// New agents expose per-peer observations. Old agents reject INSPECT, while
	// their existing permission protocol remains fully usable.
	if runtimeErr == nil && before.Generation < localapi.UpdateInspectionGeneration {
		v.Code = "peer_status_unavailable"
		return v
	}
	inspection, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "INSPECT", Peer: peer})
	if err != nil || inspection.Code != "peer_status" || inspection.PeerInfo == nil || inspection.PeerInfo.Peer != peer {
		v.Code = "peer_status_unavailable"
		return v
	}
	projectUpdateInspection(&v, *inspection.PeerInfo)
	return v
}

func updatePermission(allowed []string, peer string) string {
	for _, fp := range allowed {
		if fp == peer {
			return "ALLOWED"
		}
	}
	return "DENIED"
}
func safeUpdateBuild(build string) string {
	if build == "development" {
		return build
	}
	if len(build) != 40 && len(build) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(build); err != nil {
		return ""
	}
	return strings.ToLower(build)
}
func setLocalUpdateVersion(v *UpdateSettingsSnapshot, r localapi.RuntimeInfo) {
	if r.Generation < 1 || r.Generation > 1000000 {
		return
	}
	v.LocalGeneration = r.Generation
	v.LocalBuild = safeUpdateBuild(r.Build)
}
func projectUpdateInspection(v *UpdateSettingsSnapshot, p peerupdate.PeerInspection) {
	if p.State == "STALE" {
		v.PeerState = "STALE"
		return
	}
	if p.State != "ONLINE" || p.Generation < 21 || p.Generation > 1000000 {
		return
	}
	v.PeerState = "ONLINE"
	v.PeerGeneration = p.Generation
	v.PeerBuild = safeUpdateBuild(p.Revision)
	v.RemotePermission = "DENIED"
	if p.Allowed {
		v.RemotePermission = "ALLOWED"
	}
	if v.LocalGeneration == 0 {
		return
	}
	switch {
	case v.LocalGeneration > p.Generation:
		v.Comparison = "LOCAL_NEWER"
	case v.LocalGeneration < p.Generation:
		v.Comparison = "PEER_NEWER"
	case v.LocalBuild == "" || v.LocalBuild == "development" || v.PeerBuild == "" || v.PeerBuild == "development":
	case v.LocalBuild == v.PeerBuild:
		v.Comparison = "SAME_BUILD"
	default:
		v.Comparison = "DIFFERENT_BUILD"
	}
}
