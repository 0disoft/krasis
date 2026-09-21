package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/truststore"
)

func (o *Operations) updatePeer(fingerprint string) (truststore.PeerRecord, bool) {
	if o.trust == nil {
		return truststore.PeerRecord{}, false
	}
	doc, err := o.trust.List()
	if err != nil {
		return truststore.PeerRecord{}, false
	}
	for _, p := range doc.Peers {
		if p.Fingerprint == fingerprint && p.RevokedAt == nil {
			return p, true
		}
	}
	return truststore.PeerRecord{}, false
}
func (o *Operations) Update(ctx context.Context, r localapi.UpdateRequest) (localapi.UpdateResult, error) {
	result := localapi.UpdateResult{Schema: localapi.UpdateSchema, Code: "rejected"}
	if !r.Valid() || o.updates == nil {
		return result, nil
	}
	if r.Action == "STATUS" {
		result.Code = "ok"
		result.Status = o.updates.Status()
		return result, nil
	}
	p, ok := o.updatePeer(r.Peer)
	if !ok {
		return result, nil
	}
	if r.Action == "INSPECT" {
		info := o.updates.InspectPeer(r.Peer, time.Now())
		result.Code, result.PeerInfo = "peer_status", &info
		return result, nil
	}
	if r.Action != "DENY" && r.Action != "APPLY_FAILED" && (o.preflight == nil || o.preflight(p.Alias) != nil) {
		return result, nil
	}
	switch r.Action {
	case "APPLY_FAILED":
		o.mu.Lock()
		status := o.updates.Status()
		if o.updateApplying && status.ID == r.ID && status.Peer == r.Peer {
			o.updates.ApplyFailed()
			o.updateApplying = false
			result.Code = "apply_failed"
		}
		o.mu.Unlock()
	case "ALLOW", "DENY":
		if err := o.updates.Permit(r.Peer, r.Action == "ALLOW"); err != nil {
			return result, nil
		}
		result.Code = "permission_saved"
	case "SEND":
		o.mu.Lock()
		connected := !o.updateApplying && o.root.Err() == nil
		o.mu.Unlock()
		if !connected {
			return result, nil
		}
		if err := o.updates.Prepare(r.Peer, r.Directory); err != nil {
			result.Code = "bundle_failed"
		} else {
			result.Code = "queued"
		}
	case "APPLY":
		o.mu.Lock()
		if o.updateApplying || o.root.Err() != nil || (o.sessionDone != nil && o.peerAlias != p.Alias) {
			o.mu.Unlock()
			return result, nil
		}
		directory, _, err := o.updates.Claim(r.ID, r.Peer)
		if err != nil {
			o.mu.Unlock()
			return result, nil
		}
		o.updateApplying = true
		idle := o.sessionDone == nil
		o.mu.Unlock()
		stop := completed("no_active_session")
		if !idle {
			stop, err = o.Disconnect(ctx, localapi.OperationRequest{})
		}
		if err != nil || stop.Status != "COMPLETED" {
			o.updates.ApplyFailed()
			o.mu.Lock()
			o.updateApplying = false
			o.mu.Unlock()
			result.Code = "cleanup_failed"
			return result, nil
		}
		// Idle FAULTED is not cleared here. The new GUI must terminate the old
		// process through the verified lifecycle before starting a replacement.
		result.Code = "apply_ready"
		result.Directory = directory
		result.Alias = p.Alias
	}
	result.Status = o.updates.Status()
	return result, nil
}

func (o *Operations) updateAuthorized(ctx context.Context, alias, fp string) bool {
	if ctx.Err() != nil {
		return false
	}
	if _, ok := o.updatePeer(fp); !ok {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.root.Err() == nil && !o.updateApplying
}

// Keep a fresh certificate/trust check on every auxiliary request.
func (o *Operations) runUpdates(ctx context.Context, alias string, prepared productConnection, trust *truststore.Store) {
	if o.updates == nil {
		return
	}
	o.updates.Start(ctx, prepared.profile.Address, prepared.peer.CertificateSHA256, string(prepared.peer.Role) == "CONNECTOR", prepared.tls, func() bool {
		_, err := trust.PeerCertificate(prepared.peer, time.Now())
		profile, profileErr := trust.Connection(alias)
		return err == nil && profileErr == nil && profile.Address == prepared.profile.Address && profile.Fingerprint == prepared.profile.Fingerprint && o.updateAuthorized(ctx, alias, prepared.peer.CertificateSHA256)
	})
}
