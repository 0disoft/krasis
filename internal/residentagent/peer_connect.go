package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type peerConnectPermissionKey struct{}

// The caller can only start the connection to its own current paired record.
// Existing sessions to other peers, repair and installation stay outside this capability.
func (o *Operations) connectForPeer(ctx context.Context, fingerprint string) string {
	p, ok := o.updatePeer(fingerprint)
	if !ok || !p.AllowInboundControl || !o.peerConnectAllowed(ctx, fingerprint) {
		return "rejected"
	}
	status, err := o.Status(ctx)
	if err != nil {
		return "rejected"
	}
	if status.State == "DISCONNECTING" || (status.State == "CONNECTED" || status.State == "CONNECTING") && status.PeerAlias != p.Alias {
		return "busy"
	}
	if (status.State == "CONNECTED" || status.State == "CONNECTING") && !o.peerConnectSessionMatches(p.Alias, fingerprint) {
		return "busy"
	}
	if status.PeerAlias != p.Alias || (status.State != "CONNECTED" && status.State != "CONNECTING") {
		result, err := o.Connect(context.WithValue(ctx, peerConnectPermissionKey{}, fingerprint), localapi.OperationRequest{PeerAlias: p.Alias, Fingerprint: fingerprint})
		if err != nil {
			return "connection_unconfirmed"
		}
		if result.Status != "COMPLETED" {
			if result.Code == "already_active" {
				return "busy"
			}
			return "rejected"
		}
	}
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return "connection_unconfirmed"
		}
		if !o.peerConnectAllowed(ctx, fingerprint) {
			return "rejected"
		}
		status, err = o.Status(ctx)
		if err != nil {
			return "connection_unconfirmed"
		}
		if status.PeerAlias == p.Alias && status.State == "CONNECTED" {
			if !o.peerConnectSessionMatches(p.Alias, fingerprint) {
				return "connection_unconfirmed"
			}
			return "connected"
		}
		if status.PeerAlias != p.Alias || status.State != "CONNECTING" {
			return "connection_unconfirmed"
		}
		select {
		case <-ctx.Done():
			return "connection_unconfirmed"
		case <-tick.C:
		}
	}
}

func (o *Operations) peerConnectSessionMatches(alias, fingerprint string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.sessionDone != nil && o.peerAlias == alias && o.sessionFingerprint == fingerprint
}

func (o *Operations) peerConnectAllowed(ctx context.Context, fingerprint string) bool {
	p, ok := o.updatePeer(fingerprint)
	return ok && p.AllowInboundControl && o.updateAuthorized(ctx, p.Alias, fingerprint)
}
