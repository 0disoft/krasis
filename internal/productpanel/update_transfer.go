package productpanel

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
	"github.com/0disoft/krasis/internal/updatebundle"
)

const UpdateTransferSchema = "krasis.update-transfer/v1"

const (
	transferCallTimeout         = 15 * time.Second
	transferPeerHexLength       = 64
	transferIDHexLength         = 32
	transferBuildHexLength      = 40
	transferMinGeneration       = 21
	transferMaxGeneration       = 1000000
	transferLegacyGeneration    = 75
	transferProbeGeneration     = 90
	transferProbeSendGeneration = 91
)

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
func NewUpdateTransferBridge(ctx context.Context, open func() (UpdateSettingsClient, error), directory, transitionDirectory, probeTransitionDirectory, installerTransitionDirectory string) *UpdateSettingsBridge {
	b := NewUpdateSettingsBridge(ctx, open)
	b.bundleDirectory = directory
	b.transitionDirectory = transitionDirectory
	b.probeTransitionDirectory = probeTransitionDirectory
	b.installerTransitionDirectory = installerTransitionDirectory
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
	if len(peer) != transferPeerHexLength {
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
	ctx, cancel := context.WithTimeout(b.ctx, transferCallTimeout)
	defer cancel()
	c, err := b.open()
	if err != nil || c == nil {
		return fail("UNAVAILABLE")
	}
	return b.transferWithClient(ctx, c, peer, send)
}

func transferRuntime(ctx context.Context, c UpdateSettingsClient, peer string, send bool) (localapi.RuntimeInfo, string) {
	trust, err := c.Trust(ctx)
	if err != nil {
		return localapi.RuntimeInfo{}, "UNAVAILABLE"
	}
	trusted := false
	for _, p := range trust.Peers {
		if p.Fingerprint == peer && !p.Revoked {
			trusted = true
		}
	}
	if !trusted {
		return localapi.RuntimeInfo{}, "STALE_PEER"
	}
	runtime, err := c.Runtime(ctx)
	if err != nil || runtime.Instance == "" {
		return localapi.RuntimeInfo{}, "UNAVAILABLE"
	}
	if send && runtime.Generation != localapi.AgentGeneration {
		return localapi.RuntimeInfo{}, "AGENT_MISMATCH"
	}
	return runtime, ""
}

// The caller holds b.mu across admission, the request, and the instance recheck.
func (b *UpdateSettingsBridge) transferWithClient(ctx context.Context, c UpdateSettingsClient, peer string, send bool) UpdateTransferSnapshot {
	fail := func(state string) UpdateTransferSnapshot { return transferSnapshot(peer, state) }
	runtime, state := transferRuntime(ctx, c, peer, send)
	if state != "" {
		return fail(state)
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
	if send {
		return b.sendTransfer(ctx, c, peer, runtime, result.Status)
	}
	return b.pollTransfer(ctx, c, peer, runtime, result.Status)
}

func (b *UpdateSettingsBridge) pollTransfer(ctx context.Context, c UpdateSettingsClient, peer string, runtime localapi.RuntimeInfo, status peerupdate.Status) UpdateTransferSnapshot {
	if b.transfer.ID == "" {
		inspection, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "INSPECT", Peer: peer})
		if err == nil && transferInspectionMatches(inspection, peer) && peerBuildResult(runtime, inspection.PeerInfo.Generation, inspection.PeerInfo.Revision) == "SAME_VERSION" {
			b.transfer = transferSnapshot(peer, "SAME_VERSION")
		}
		return b.transfer
	}
	if status.Peer != peer || status.ID != b.transfer.ID {
		return transferSnapshot(peer, "TRANSFER_UNKNOWN")
	}
	b.transfer = projectTransfer(peer, status, status.State)
	return b.confirmTransferRuntime(ctx, c, peer, runtime.Instance)
}

func (b *UpdateSettingsBridge) prepareTransferSend(ctx context.Context, c UpdateSettingsClient, peer string, runtime localapi.RuntimeInfo, status peerupdate.Status) (string, string) {
	if b.bundleDirectory == "" {
		return "", "BUNDLE_MISSING"
	}
	if updateTransferActive(status.State) {
		return "", "PEER_BUSY"
	}
	inspection, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "INSPECT", Peer: peer})
	if err != nil || !transferInspectionMatches(inspection, peer) || inspection.PeerInfo.Generation < transferMinGeneration || inspection.PeerInfo.Generation > transferMaxGeneration {
		return "", "PEER_UNREACHABLE"
	}
	if state := peerBuildResult(runtime, inspection.PeerInfo.Generation, inspection.PeerInfo.Revision); state != "" {
		return "", state
	}
	directory := b.transferDirectory(runtime.Generation, inspection.PeerInfo.Generation)
	if directory == "" {
		return "", "TRANSITION_REQUIRED"
	}
	return directory, ""
}

func (b *UpdateSettingsBridge) sendTransfer(ctx context.Context, c UpdateSettingsClient, peer string, runtime localapi.RuntimeInfo, status peerupdate.Status) UpdateTransferSnapshot {
	directory, state := b.prepareTransferSend(ctx, c, peer, runtime, status)
	if state != "" {
		return transferSnapshot(peer, state)
	}
	// Record uncertainty before I/O: a lost reply must never trigger a resend.
	b.transfer = transferSnapshot(peer, "TRANSFER_UNKNOWN")
	b.transferInstance = runtime.Instance
	result, err := c.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "SEND", Peer: peer, Directory: directory})
	if err != nil {
		return b.transfer
	}
	next, confirm := projectTransferSend(peer, result)
	b.transfer = next
	if !confirm {
		return b.transfer
	}
	return b.confirmTransferRuntime(ctx, c, peer, runtime.Instance)
}

func (b *UpdateSettingsBridge) confirmTransferRuntime(ctx context.Context, c UpdateSettingsClient, peer, instance string) UpdateTransferSnapshot {
	after, err := c.Runtime(ctx)
	if err != nil || after.Instance != instance {
		return transferSnapshot(peer, "AGENT_CHANGED")
	}
	return b.transfer
}

func transferInspectionMatches(result localapi.UpdateResult, peer string) bool {
	return result.Code == "peer_status" && result.PeerInfo != nil && result.PeerInfo.Peer == peer && result.PeerInfo.State == "ONLINE"
}

// projectTransferSend does not mutate result; confirm requests a runtime recheck.
func projectTransferSend(peer string, result localapi.UpdateResult) (next UpdateTransferSnapshot, confirm bool) {
	if result.Code == "bundle_failed" && result.Status.Peer == peer {
		return projectTransfer(peer, result.Status, result.Status.State), false
	}
	if result.Code != "queued" {
		return transferSnapshot(peer, "REJECTED"), false
	}
	if result.Status.Peer != peer || !validTransferID(result.Status.ID) {
		return transferSnapshot(peer, "TRANSFER_UNKNOWN"), false
	}
	return projectTransfer(peer, result.Status, result.Status.State), true
}

func (b *UpdateSettingsBridge) transferDirectory(localGeneration, peerGeneration int) string {
	if peerGeneration < transferLegacyGeneration {
		return b.transitionDirectory
	}
	if peerGeneration < transferProbeGeneration && localGeneration >= transferProbeSendGeneration {
		return b.probeTransitionDirectory
	}
	if peerGeneration < updatebundle.InstallerLayoutGeneration && localGeneration >= updatebundle.InstallerBundleGeneration {
		return b.installerTransitionDirectory
	}
	return b.bundleDirectory
}

func peerBuildResult(local localapi.RuntimeInfo, generation int, revision string) string {
	if generation > local.Generation {
		return "PEER_NEWER"
	}
	localBuild, peerBuild := safeUpdateBuild(local.Build), safeUpdateBuild(revision)
	if generation != local.Generation || len(localBuild) != transferBuildHexLength || len(peerBuild) != transferBuildHexLength {
		return ""
	}
	if localBuild == peerBuild {
		return "SAME_VERSION"
	}
	return "GENERATION_CONFLICT"
}

func validTransferID(id string) bool {
	if len(id) != transferIDHexLength {
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
	if s.Total > 0 && s.Total <= updatebundle.MaxArchiveBytes && s.Bytes >= 0 && s.Bytes <= s.Total {
		v.Bytes = s.Bytes
		v.Total = s.Total
	}
	if s.Generation >= transferMinGeneration && s.Generation <= transferMaxGeneration {
		v.Generation = s.Generation
		v.Build = safeUpdateBuild(s.Revision)
	}
	return v
}
