package productpanel

import (
	"context"
	"errors"
	"time"
)

type pendingInputRecovery struct {
	alias, fingerprint string
	startup            bool
	deadline           time.Time
}

// Ensure runs under its normal startup ownership fence. The callback only
// verifies installation completion; it cannot launch another installer.
func (s Startup) RestoreProtectedInput(ctx context.Context) error {
	if s.InputRestoreCheck == nil {
		return ErrInputRefreshPending
	}
	restore := s
	restore.BeforeStart = func(context.Context) error { return s.InputRestoreCheck() }
	return restore.Ensure(ctx)
}

// Caller owns the lifecycle gate. Consume before starting/resuming so a lost
// reply or another poll cannot replay this continuation.
func (b *DesktopLifecycle) continuePendingInput(ctx context.Context, now time.Time, value BridgeSnapshot) (BridgeSnapshot, bool) {
	p := b.pendingInput
	if p == nil {
		return value, false
	}
	previous := b.Refresh()
	value.InputRefreshCode, value.ReconnectCode = previous.InputRefreshCode, previous.ReconnectCode
	if b.stopped.Load() || b.ctx.Err() != nil || ctx.Err() != nil || b.updateHandedOff {
		b.pendingInput = nil
		value.Code = "cancelled"
		return value, true
	}
	if !now.Before(p.deadline) {
		b.pendingInput, b.updatedResume = nil, nil
		value.Code = "input_refresh_pending"
		return value, true
	}
	if b.PendingInputSetup == nil || b.PendingInputRestore == nil {
		return value, true
	}
	setup := b.PendingInputSetup()
	if setup != "READY" && setup != "UPDATE_REQUIRED" && setup != "NOT_INSTALLED" {
		value.Code = "input_refresh_pending"
		return value, true
	}
	b.pendingInput = nil
	err := b.PendingInputRestore(ctx)
	if errors.Is(err, ErrInputRefreshPending) {
		b.pendingInput = p // A fresh check found another running/unconfirmed transaction.
		value.Code = "input_refresh_pending"
		return value, true
	}
	if err != nil {
		b.updatedResume = nil
		value.Code = "startup_failed"
		return value, true
	}
	if b.stopped.Load() || b.ctx.Err() != nil || ctx.Err() != nil {
		value.Code = "cancelled"
		return value, true
	}
	if b.StartupReady != nil {
		b.StartupReady()
	}
	bridge := NewDesktopBridge(ctx, b.open)
	value = bridge.Refresh()
	refreshCode, reconnectCode := "ok", "not_attempted"
	if b.PendingInputSetup() != "READY" {
		refreshCode = "input_refresh_failed"
		b.updatedResume = nil
	}
	if p.alias != "" && value.Code == "ok" && value.State == "LOCAL" {
		value = b.recoveryResume(bridge, p.alias, p.fingerprint)
		reconnectCode = value.Code
	} else if p.startup && p.alias == "" && b.StartupResume != nil {
		value = b.StartupResume(ctx, value)
	}
	value.InputRefreshCode, value.ReconnectCode = refreshCode, reconnectCode
	b.startUpdatedResumeWindow(now)
	return value, true
}
