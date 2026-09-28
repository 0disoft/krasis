package productpanel

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

const UpdateTransferSchema = "krasis.update-transfer/v1"

type UpdateTransferSnapshot struct {
	Schema     string `json:"schema"`
	Peer       string `json:"peer"`
	ID         string `json:"id"`
	State      string `json:"state"`
	Bytes      int64  `json:"bytes"`
	Total      int64  `json:"total"`
	Generation int    `json:"generation"`
	Build      string `json:"build"`
}

// The directory comes from the native executable location, never the renderer.
func NewUpdateTransferBridge(ctx context.Context, open func() (UpdateSettingsClient, error), directory, transitionDirectory string) *UpdateSettingsBridge {
	b := NewUpdateSettingsBridge(ctx, open)
	b.bundleDirectory = directory
	b.transitionDirectory = transitionDirectory
	return b
}

func transferSnapshot(peer, state string) UpdateTransferSnapshot {
	return UpdateTransferSnapshot{Schema: UpdateTransferSchema, Peer: peer, State: state}
}

func (b *UpdateSettingsBridge) TransferStatus(peer string) UpdateTransferSnapshot {
	return b.transferCall(peer, false)
}
func (b *UpdateSettingsBridge) Send(peer string) UpdateTransferSnapshot {
	return b.transferCall(peer, true)
}

func updateTransferActive(state string) bool {
	switch state {
	case "PREPARING", "QUEUED", "SENDING", "VERIFYING", "DELIVERED", "RECEIVING", "STAGED", "READY", "APPLYING":
		return true
	}
	return false
}

func (b *UpdateSettingsBridge) transferCall(peer string, send bool) UpdateTransferSnapshot {
	fail := func(state string) UpdateTransferSnapshot { return transferSnapshot(peer, state) }
	if len(peer) != 64 {
		return fail("INVALID_PEER")
	}
	if _, err := hex.DecodeString(peer); err != nil {
		return fail("INVALID_PEER")
	}
	if b == nil || b.ctx == nil || b.open == nil {
		return fail("UNAVAILABLE")
	}
	if !b.mu.TryLock() {
		return fail("BUSY")
	}
	defer b.mu.Unlock()
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()
	c, err := b.open()
	if err != nil || c == nil {
		return fail("UNAVAILABLE")
	}
	trust, err := c.Trust(ctx)
	if err != nil {
		return fail("UNAVAILABLE")
	}
	trusted := false
	for _, p := range trust.Peers {
		if p.Fingerprint == peer && !p.Revoked {
			trusted = true
		}
	}
	if !trusted {
		return fail("STALE_PEER")
	}
	runtime, err := c.Runtime(ctx)
	if err != nil || runtime.Instance == "" {
		return fail("UNAVAILABLE")
	}
	if send && runtime.Generation != localapi.AgentGeneration {
		return fail("AGENT_MISMATCH")
	}
	if !send && b.transfer.Peer != peer {
		return fail("IDLE")
	}
	if !send && runtime.Instance != b.transferInstance {
		return fail("AGENT_CHANGED")
	}
	if !send && b.transfer.ID == "" && b.transfer.State != "TRANSFER_UNKNOWN" {
		return b.transfer
	}
	result, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	if err != nil || result.Code != "ok" {
		return fail("UNAVAILABLE")
	}
	if !send && b.transfer.ID == "" {
		inspection, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "INSPECT", Peer: peer})
		if err == nil && inspection.Code == "peer_status" && inspection.PeerInfo != nil && inspection.PeerInfo.Peer == peer && inspection.PeerInfo.State == "ONLINE" && peerBuildResult(runtime, inspection.PeerInfo.Generation, inspection.PeerInfo.Revision) == "SAME_VERSION" {
			b.transfer = fail("SAME_VERSION")
		}
		return b.transfer
	}
	if send {
		if b.bundleDirectory == "" {
			return fail("BUNDLE_MISSING")
		}
		if updateTransferActive(result.Status.State) {
			return fail("PEER_BUSY")
		}
		inspection, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "INSPECT", Peer: peer})
		if err != nil || inspection.Code != "peer_status" || inspection.PeerInfo == nil || inspection.PeerInfo.Peer != peer || inspection.PeerInfo.State != "ONLINE" || inspection.PeerInfo.Generation < 21 || inspection.PeerInfo.Generation > 1000000 {
			return fail("PEER_UNREACHABLE")
		}
		if state := peerBuildResult(runtime, inspection.PeerInfo.Generation, inspection.PeerInfo.Revision); state != "" {
			return fail(state)
		}
		directory := b.bundleDirectory
		if inspection.PeerInfo.Generation < 75 {
			if b.transitionDirectory == "" {
				return fail("TRANSITION_REQUIRED")
			}
			directory = b.transitionDirectory
		}
		b.transfer = fail("TRANSFER_UNKNOWN")
		b.transferInstance = runtime.Instance
		result, err = c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "SEND", Peer: peer, Directory: directory})
		if err != nil {
			return b.transfer
		} // Never retry a request whose outcome is unknown.
		if result.Code != "queued" {
			b.transfer = fail("REJECTED")
			if result.Code == "bundle_failed" && result.Status.Peer == peer {
				b.transfer = projectTransfer(peer, result.Status, result.Status.State)
			}
			return b.transfer
		}
		if result.Status.Peer != peer || !validTransferID(result.Status.ID) {
			return b.transfer
		}
		b.transfer = projectTransfer(peer, result.Status, result.Status.State)
	} else {
		if result.Status.Peer != peer || result.Status.ID != b.transfer.ID {
			return fail("TRANSFER_UNKNOWN")
		}
		b.transfer = projectTransfer(peer, result.Status, result.Status.State)
	}
	after, err := c.Runtime(ctx)
	if err != nil || after.Instance != runtime.Instance {
		return fail("AGENT_CHANGED")
	}
	return b.transfer
}

func peerBuildResult(local localapi.RuntimeInfo, generation int, revision string) string {
	if generation > local.Generation {
		return "PEER_NEWER"
	}
	localBuild, peerBuild := safeUpdateBuild(local.Build), safeUpdateBuild(revision)
	if generation != local.Generation || len(localBuild) != 40 || len(peerBuild) != 40 {
		return ""
	}
	if localBuild == peerBuild {
		return "SAME_VERSION"
	}
	return "GENERATION_CONFLICT"
}

func validTransferID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func projectTransfer(peer string, s peerupdate.Status, state string) UpdateTransferSnapshot {
	v := transferSnapshot(peer, "TRANSFER_UNKNOWN")
	switch state {
	case "PREPARING", "QUEUED", "SENDING", "VERIFYING", "DELIVERED", "UPDATED", "RESTART_UNCONFIRMED", "SAME_VERSION", "PEER_NEWER", "GENERATION_CONFLICT", "PEER_BUSY", "PEER_UNREACHABLE", "PEER_UNSUPPORTED", "PEER_NOT_ALLOWED", "INVALID_BUNDLE", "BUNDLE_MISSING", "TRANSITION_REQUIRED", "HASH_MISMATCH", "STORAGE_FAILED", "TRANSFER_INTERRUPTED", "PEER_APPLY_FAILED", "EXPIRED", "PEER_REJECTED", "TRANSFER_UNKNOWN", "REJECTED", "AGENT_MISMATCH":
		v.State = state
	}
	if validTransferID(s.ID) {
		v.ID = s.ID
	}
	if s.Total > 0 && s.Total <= 96<<20 && s.Bytes >= 0 && s.Bytes <= s.Total {
		v.Bytes = s.Bytes
		v.Total = s.Total
	}
	if s.Generation >= 21 && s.Generation <= 1000000 {
		v.Generation = s.Generation
		v.Build = safeUpdateBuild(s.Revision)
	}
	return v
}
