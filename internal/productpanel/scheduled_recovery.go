package productpanel

import (
	"context"
	"github.com/0disoft/krasis/internal/appsettings"
)

// A recovery request is distinct from showing the UI. Ensure rechecks the
// agent instance lease and bundle under its startup ownership lock.
func (b *DesktopLifecycle) ScheduledRecover(allowed func() bool, resume func(context.Context, BridgeSnapshot) BridgeSnapshot) {
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
	if err := b.startup(ctx); err != nil {
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

func RecoveryAllowed(base string) bool {
	settings, err := appsettings.Load(base)
	if err != nil || !settings.StartAtLogin {
		return false
	}
	paused, err := appsettings.RecoveryPaused(base)
	return err == nil && !paused
}
