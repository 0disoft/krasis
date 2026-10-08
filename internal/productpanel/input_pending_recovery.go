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
	restoreAfter       time.Time
	restored           bool
	refreshCode        string
}

// Ensure runs under its normal startup ownership fence. The callback only
// verifies installation completion; it cannot launch another installer.
func (s Startup) RestoreProtectedInput(ctx context.Context) error {
	if s.InputRestoreCheck == nil {
		return ErrInputRefreshPending
	}
	if err := s.InputRestoreCheck(); err != nil {
		return err // Also fence an already running agent against stale completion health.
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
		if p.restored {
			value.InputRefreshCode = p.refreshCode
		}
		// Expiring this continuation says nothing about installation success.
		// Preserve its outcome separately from the completed health observation.
		value.Code, value.ReconnectCode = "input_refresh_wait_expired", "input_refresh_wait_expired"
		return value, true
	}
	if !p.restored {
		if now.Before(p.restoreAfter) {
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
		restoreCtx, cancel := context.WithDeadline(ctx, p.deadline)
		err := b.PendingInputRestore(restoreCtx)
		cancel()
		if errors.Is(err, ErrInputRefreshPending) {
			value.Code = "input_refresh_pending"
			return value, true
		}
		if errors.Is(err, ErrStartupBusy) {
			// Ensure did not acquire startup ownership or start an agent.
			// Retain the same request/deadline and avoid retrying on every poll.
			p.restoreAfter = now.Add(5 * time.Second)
			value.Code = "input_refresh_pending"
			return value, true
		}
		if err != nil {
			b.pendingInput, b.updatedResume = nil, nil
			value.Code = "startup_failed"
			return value, true
		}
		p.restored, p.refreshCode = true, "ok"
		if b.PendingInputSetup() != "READY" {
			p.refreshCode = "input_refresh_failed"
			b.updatedResume = nil
		}
		if b.stopped.Load() || b.ctx.Err() != nil || ctx.Err() != nil {
			b.pendingInput = nil
			value.Code = "cancelled"
			return value, true
		}
		if b.StartupReady != nil {
			b.StartupReady()
		}
		value = NewDesktopBridge(ctx, b.open).Refresh()
	}
	value.InputRefreshCode, value.ReconnectCode = p.refreshCode, "not_attempted"
	// Read-only confirmation retains the same identity/deadline. Never prepare
	// another agent because a status or trust read after restoration failed.
	if value.Code != "ok" {
		if value.Code == "recovery_required" {
			b.pendingInput, b.updatedResume = nil, nil
		}
		return value, true
	}
	if p.alias != "" {
		matches := 0
		changed := false
		for _, peer := range value.Peers {
			if peer.Alias == p.alias {
				matches++
				changed = changed || peer.Fingerprint != p.fingerprint
			}
		}
		if matches != 1 || changed || value.PeerAlias != "" && value.PeerAlias != p.alias {
			b.pendingInput, b.updatedResume = nil, nil
			value.Code, value.ReconnectCode = "stale_peer", "stale_peer"
			return value, true
		}
	}
	switch value.State {
	case "UNKNOWN", "CONNECTING", "RECONNECTING":
		return value, true
	case "LOCAL", "CONNECTED":
	default:
		b.pendingInput, b.updatedResume = nil, nil
		value.Code = "agent_busy"
		return value, true
	}
	if value.State == "LOCAL" {
		if p.alias != "" {
			value = b.recoveryResume(NewDesktopBridge(ctx, b.open), p.alias, p.fingerprint)
			value.ReconnectCode = value.Code
		} else if p.startup && b.StartupResume != nil {
			value = b.StartupResume(ctx, value)
		}
		value.InputRefreshCode = p.refreshCode
		if value.ReconnectCode == "" {
			value.ReconnectCode = "not_attempted"
		}
		// The bridge reserves outcome_unknown for possibly sent commands.
		// Only verified pre-send read failures can wait for another observation.
		if value.Code == "unavailable" || value.Code == "busy" {
			return value, true
		}
	}
	b.pendingInput = nil
	b.startUpdatedResumeWindow(now)
	return value, true
}
