package productpanel

import "context"

// A recovery request is distinct from showing the UI. Ensure rechecks the
// agent instance lease and bundle under its startup ownership lock.
func (b *DesktopLifecycle) ScheduledRecover(allowed func() bool, ensure func(context.Context) error, resume func(context.Context, BridgeSnapshot) BridgeSnapshot) {
	if allowed == nil || !b.gate.TryLock() {
		return
	}
	defer b.gate.Unlock()
	if b.stopped.Load() || b.updateHandedOff || !allowed() {
		return
	}
	ctx, done := b.operation()
	defer done()
	value := NewDesktopBridge(ctx, b.open).Refresh()
	if value.State != "UNKNOWN" || value.Code != "unavailable" {
		return // Responding, faulted or unverified agents are not absent.
	}
	if ensure == nil {
		ensure = b.ensure
	}
	if ensure == nil {
		return
	}
	if err := ensure(ctx); err != nil {
		b.failedStartup(ctx, err)
		return
	}
	if b.stopped.Load() || ctx.Err() != nil || !allowed() {
		return
	}
	value = NewDesktopBridge(ctx, b.open).Refresh()
	if resume != nil {
		value = resume(ctx, value)
	}
	b.save(value)
}
