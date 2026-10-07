package productpanel

import (
	"context"
	"time"
)

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
	if !start {
		record := *b.collection
		worker.collection = &record
	}
	go func() {
		budget := 3 * time.Second
		if start {
			budget = 10 * time.Second
		}
		ctx, cancel := context.WithTimeout(b.lifetime, budget)
		defer cancel()
		if start {
			worker.collectConnected(ctx)
		} else {
			worker.pollCollection(ctx)
		}
		b.mu.Lock()
		b.collection = worker.collection
		b.working = false
		b.mu.Unlock()
	}()
}
