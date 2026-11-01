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

const (
	updateSettingsTimeout      = 3 * time.Second
	updateFingerprintHexLength = 64
	updateBuildSHA1HexLength   = 40
	updateBuildSHA256HexLength = 64
	updateMinLocalGeneration   = 1
	updateMinPeerGeneration    = 21
	updateMaxGeneration        = 1000000
)

type UpdateSettingsSnapshot struct {
	Schema           string `json:"schema"`
	Code             string `json:"code"`
	Peer             string `json:"peer"`
	Permission       string `json:"permission"`
	RemotePermission string `json:"remotePermission"`
	PeerState        string `json:"peerState"`
	AppGeneration    int    `json:"appGeneration"`
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
	ctx                          context.Context
	open                         func() (UpdateSettingsClient, error)
	mu                           sync.Mutex
	bundleDirectory              string
	transitionDirectory          string
	probeTransitionDirectory     string
	installerTransitionDirectory string
	transfer                     UpdateTransferSnapshot
	transferInstance             string
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
	return UpdateSettingsSnapshot{Schema: UpdateSettingsSchema, Code: code, Peer: peer, Permission: "UNKNOWN", RemotePermission: "UNKNOWN", PeerState: "UNKNOWN", AppGeneration: localapi.AgentGeneration, Comparison: "UNKNOWN"}
}
func (b *UpdateSettingsBridge) execute(peer, action string) UpdateSettingsSnapshot {
	v := updateSettingsFailure("", "unavailable")
	if b == nil || b.ctx == nil || b.open == nil {
		return v
	}
	if len(peer) != updateFingerprintHexLength {
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
	ctx, cancel := context.WithTimeout(b.ctx, updateSettingsTimeout)
	defer cancel()
	if ctx.Err() != nil {
		v.Code = "cancelled"
		return v
	}
	c, err := b.open()
	if err != nil || c == nil {
		return v
	}
	return executeUpdateSettings(ctx, c, v, action)
}

// execute holds the bridge lock throughout trust, the request and confirmation.
func executeUpdateSettings(ctx context.Context, c UpdateSettingsClient, v UpdateSettingsSnapshot, action string) UpdateSettingsSnapshot {
	trust, err := c.Trust(ctx)
	if err != nil {
		return v
	}
	trusted := false
	for _, p := range trust.Peers {
		if p.Fingerprint == v.Peer && !p.Revoked {
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
		v = withLocalUpdateVersion(v, before)
	}
	r := localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: action}
	if action != "STATUS" {
		r.Peer = v.Peer
	}
	if ctx.Err() != nil {
		v.Code = "cancelled"
		return v
	}
	result, err := c.Update(ctx, r)
	v, accepted := projectUpdateSettingsReply(v, action, result, err)
	if !accepted {
		return v
	}
	if action != "STATUS" {
		return confirmUpdatePermission(ctx, c, v, action, before, runtimeErr)
	}
	return inspectUpdateSettings(ctx, c, v, before, runtimeErr)
}

// Project a reply without changing the caller's snapshot or permission list.
func projectUpdateSettingsReply(v UpdateSettingsSnapshot, action string, result localapi.UpdateResult, err error) (UpdateSettingsSnapshot, bool) {
	if err != nil {
		if action != "STATUS" {
			v.Code = "outcome_unknown"
		}
		return v, false
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
		return v, false
	}
	v.Permission = updatePermission(result.Status.Allowed, v.Peer)
	v.Code = expected
	return v, true
}

func confirmUpdatePermission(ctx context.Context, c UpdateSettingsClient, v UpdateSettingsSnapshot, action string, before localapi.RuntimeInfo, runtimeErr error) UpdateSettingsSnapshot {
	if runtimeErr != nil || before.Instance == "" {
		return projectUpdatePermissionAck(v, action)
	}
	after, err := c.Runtime(ctx)
	if err != nil || after.Instance != before.Instance {
		return updateSettingsFailure(v.Peer, "agent_changed")
	}
	return projectUpdatePermissionAck(v, action)
}

func projectUpdatePermissionAck(v UpdateSettingsSnapshot, action string) UpdateSettingsSnapshot {
	if (action == "ALLOW") != (v.Permission == "ALLOWED") {
		v.Permission = "UNKNOWN"
		v.Code = "outcome_unknown"
	}
	return v
}

func inspectUpdateSettings(ctx context.Context, c UpdateSettingsClient, v UpdateSettingsSnapshot, before localapi.RuntimeInfo, runtimeErr error) UpdateSettingsSnapshot {
	// New agents expose per-peer observations. Old agents reject INSPECT, while
	// their existing permission protocol remains fully usable.
	if runtimeErr == nil && before.Generation < localapi.UpdateInspectionGeneration {
		v.Code = "peer_status_unavailable"
		return v
	}
	inspection, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "INSPECT", Peer: v.Peer})
	if err != nil || inspection.Code != "peer_status" || inspection.PeerInfo == nil || inspection.PeerInfo.Peer != v.Peer {
		v.Code = "peer_status_unavailable"
		return v
	}
	return projectUpdateInspection(v, *inspection.PeerInfo)
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
	if len(build) != updateBuildSHA1HexLength && len(build) != updateBuildSHA256HexLength {
		return ""
	}
	if _, err := hex.DecodeString(build); err != nil {
		return ""
	}
	return strings.ToLower(build)
}
func withLocalUpdateVersion(v UpdateSettingsSnapshot, r localapi.RuntimeInfo) UpdateSettingsSnapshot {
	if r.Generation < updateMinLocalGeneration || r.Generation > updateMaxGeneration {
		return v
	}
	v.LocalGeneration = r.Generation
	v.LocalBuild = safeUpdateBuild(r.Build)
	return v
}
func projectUpdateInspection(v UpdateSettingsSnapshot, p peerupdate.PeerInspection) UpdateSettingsSnapshot {
	if p.State == "STALE" {
		v.PeerState = "STALE"
		return v
	}
	if p.State != "ONLINE" || p.Generation < updateMinPeerGeneration || p.Generation > updateMaxGeneration {
		return v
	}
	v.PeerState = "ONLINE"
	v.PeerGeneration = p.Generation
	v.PeerBuild = safeUpdateBuild(p.Revision)
	v.RemotePermission = "DENIED"
	if p.Allowed {
		v.RemotePermission = "ALLOWED"
	}
	if v.LocalGeneration == 0 {
		return v
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
	return v
}
