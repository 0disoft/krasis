package productpanel

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/productlog"
)

// ConfigureCollectionStorage is wired once by the native host before bindings
// are exposed. Restoring a receipt never sends START, including after a crash.
func (b *DiagnosticsBridge) ConfigureCollectionStorage(load func() (*productlog.CollectionReceipt, error), save func(productlog.CollectionReceipt) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.async, b.saveCollection = true, save
	r, err := load()
	if err != nil {
		b.collection = &diagnosticCollection{code: "state_save_failed", view: DiagnosticCollectionSnapshot{State: "UNKNOWN"}}
		return
	}
	if r == nil {
		return
	}
	b.collection = &diagnosticCollection{peer: r.Peer, id: r.ID, expires: r.Expires, code: r.Code,
		view: DiagnosticCollectionSnapshot{State: r.State, LocalSaved: r.LocalSaved, PeerSaved: r.PeerSaved}}
	if r.State == "SAVING" {
		// The request may have reached the agent before the app disappeared.
		b.collection.code, b.collection.view.State = "outcome_unknown", "UNKNOWN"
	}
}

func (r *diagnosticCollection) receipt() productlog.CollectionReceipt {
	return productlog.CollectionReceipt{Schema: "krasis.diagnostic-collection/v1", ID: r.id, Peer: r.peer,
		Expires: r.expires, State: r.view.State, Code: r.code, LocalSaved: r.view.LocalSaved, PeerSaved: r.view.PeerSaved}
}

// BeginCollect is the desktop entry point: neither saving logs nor waiting on
// the agent occupies the renderer call. At most one native worker is active.
func (b *DiagnosticsBridge) BeginCollect() DiagnosticsSnapshot {
	if b == nil || b.lifetime == nil || b.open == nil || b.export == nil {
		return DiagnosticsSnapshot{Schema: DiagnosticsSchema, Code: "unavailable", Transfer: "UNKNOWN"}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.lifetime.Err() != nil {
		return DiagnosticsSnapshot{Schema: DiagnosticsSchema, Code: "cancelled", Transfer: "UNKNOWN"}
	}
	b.async = true
	if b.working || b.collection != nil && b.collection.view.State == "PENDING" {
		b.startCollectionWorker(false)
		return b.collection.snapshot()
	}
	b.collection = &diagnosticCollection{code: "saving", view: DiagnosticCollectionSnapshot{State: "SAVING"}}
	b.startCollectionWorker(true)
	return b.collection.snapshot()
}

// Called with mu held. The worker owns its private record until publication;
// status calls only read a copy and cannot race with file or RPC completion.
func (b *DiagnosticsBridge) startCollectionWorker(start bool) {
	if b.working || b.lifetime.Err() != nil || !start && b.collection.view.State != "PENDING" {
		return
	}
	b.working = true
	worker := &DiagnosticsBridge{lifetime: b.lifetime, open: b.open, export: b.export}
	record := *b.collection
	worker.collection = &record
	save := b.saveCollection
	go func() {
		budget := 3 * time.Second
		if start {
			budget = 10 * time.Second
		}
		ctx, cancel := context.WithTimeout(b.lifetime, budget)
		defer cancel()
		before := worker.collection.receipt()
		if start && save != nil && save(before) != nil {
			worker.collection.code, worker.collection.view.State = "state_save_failed", "FAILED"
			worker.collection.view.LocalSaved = worker.export() == nil
		} else if start {
			worker.collectConnected(ctx)
		} else {
			worker.pollCollection(ctx)
		}
		after := worker.collection.receipt()
		if save != nil && after != before {
			if save(after) != nil {
				worker.collection.code = "state_save_failed"
			}
		}
		b.mu.Lock()
		b.collection = worker.collection
		b.working = false
		b.mu.Unlock()
	}()
}
