package productpanel

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

// These booleans describe this request's files, never older exports on disk.
type DiagnosticCollectionSnapshot struct {
	State      string `json:"state"`
	LocalSaved bool   `json:"localSaved"`
	PeerSaved  bool   `json:"peerSaved"`
}

type diagnosticCollectionClient interface {
	Status(context.Context) (localapi.Status, error)
	CollectDiagnostic(context.Context, localapi.DiagnosticCollectionRequest) (localapi.DiagnosticCollectionResult, error)
}

type diagnosticCollection struct {
	peer, id, code string
	expires        time.Time
	view           DiagnosticCollectionSnapshot
}

func (r *diagnosticCollection) snapshot() DiagnosticsSnapshot {
	view := r.view
	return DiagnosticsSnapshot{Schema: DiagnosticsSchema, Code: r.code, Saved: view.LocalSaved,
		Received: view.PeerSaved, Transfer: "NONE", Collection: &view}
}

// The native agent chooses the active peer; the renderer supplies no identity,
// paths or command. Retain the exact request ID and never retry START on a poll.
func (b *DiagnosticsBridge) collectConnected(ctx context.Context) (snapshot DiagnosticsSnapshot) {
	if b.collection != nil && b.collection.view.State == "PENDING" {
		return b.pollCollection(ctx)
	}
	r := &diagnosticCollection{code: "partial", view: DiagnosticCollectionSnapshot{State: "FAILED"}}
	b.collection = r
	// START writes the immutable local snapshot itself. Export a fallback only
	// when that write was not confirmed; never write the logs twice on success.
	defer func() {
		if !r.view.LocalSaved {
			if b.export() != nil {
				r.code, r.view.State = "save_failed", "FAILED"
			} else {
				r.view.LocalSaved = true
			}
		}
		snapshot = r.snapshot()
	}()
	if ctx.Err() != nil {
		r.code = "cancelled"
		return r.snapshot()
	}
	client, err := b.open()
	if err != nil || client == nil {
		return r.snapshot()
	}
	api, ok := client.(diagnosticCollectionClient)
	if !ok {
		r.code = "unsupported"
		return r.snapshot()
	}
	status, err := api.Status(ctx)
	if err != nil {
		return r.snapshot()
	}
	if status.State == "LOCAL" {
		r.code, r.view.State = "local_only", "LOCAL_ONLY"
		return r.snapshot()
	}
	if (status.State != "CONNECTED" && status.State != "CONNECTING" && status.State != "RECONNECTING") || status.PeerAlias == "" {
		return r.snapshot()
	}
	trust, err := client.Trust(ctx)
	if err != nil {
		return r.snapshot()
	}
	for _, peer := range trust.Peers {
		if peer.Alias != status.PeerAlias || peer.Revoked {
			continue
		}
		decoded, err := hex.DecodeString(peer.Fingerprint)
		if err != nil || len(decoded) != 32 || r.peer != "" {
			r.code = "stale_peer"
			return r.snapshot()
		}
		r.peer = peer.Fingerprint
	}
	if r.peer == "" {
		r.code = "stale_peer"
		return r.snapshot()
	}
	if ctx.Err() != nil {
		r.code = "cancelled"
		return r.snapshot()
	}
	started, err := api.CollectDiagnostic(ctx, localapi.DiagnosticCollectionRequest{Action: "START", Peer: r.peer})
	if err != nil {
		r.code, r.view.State = "outcome_unknown", "UNKNOWN"
		return r.snapshot()
	}
	if started.Code != "queued" {
		switch started.Code {
		case "unsupported", "rejected", "busy", "save_failed", "storage_limit":
			r.code = started.Code
		default:
			r.code = "outcome_unknown"
		}
		return r.snapshot()
	}
	if !validTransferID(started.ID) || started.LocalPath == "" {
		r.code, r.view.State = "outcome_unknown", "UNKNOWN"
		return r.snapshot()
	}
	r.id, r.code, r.view.State = started.ID, "pending", "PENDING"
	r.view.LocalSaved = true
	r.expires = time.Now().Add(65 * time.Second)
	return r.snapshot()
}

func (b *DiagnosticsBridge) pollCollection(ctx context.Context) DiagnosticsSnapshot {
	r := b.collection
	if r.view.State != "PENDING" {
		return r.snapshot()
	}
	expired := !time.Now().Before(r.expires)
	// A hidden window may resume polling after the wait limit. Check once for
	// an already completed receipt before declaring a partial result.
	unknown := func() DiagnosticsSnapshot {
		r.code = "outcome_unknown"
		if expired {
			r.code, r.view.State = "partial", "EXPIRED"
		}
		return r.snapshot()
	}
	client, err := b.open()
	if err != nil || client == nil {
		return unknown()
	}
	api, ok := client.(diagnosticCollectionClient)
	if !ok {
		r.code, r.view.State = "unsupported", "FAILED"
		return r.snapshot()
	}
	result, err := api.CollectDiagnostic(ctx, localapi.DiagnosticCollectionRequest{Action: "STATUS", Peer: r.peer, ID: r.id})
	if err != nil {
		return unknown()
	}
	if result.Code != "ok" || result.ID != r.id {
		r.code, r.view.State = "partial", "UNKNOWN"
		return r.snapshot()
	}
	switch result.State {
	case "RECEIVED":
		if result.LocalPath == "" || result.PeerPath == "" {
			r.code, r.view.State = "partial", "UNKNOWN"
			break
		}
		r.code, r.view.State, r.view.PeerSaved = "collected", "RECEIVED", true
	case "PENDING":
		r.code = "pending"
		if expired {
			r.code, r.view.State = "partial", "EXPIRED"
		}
	case "EXPIRED":
		r.code, r.view.State = "partial", "EXPIRED"
	default:
		r.code, r.view.State = "partial", "UNKNOWN"
	}
	return r.snapshot()
}
