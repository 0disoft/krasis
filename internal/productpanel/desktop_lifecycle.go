package productpanel

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// DesktopLifecycle owns recovery independently of renderer timers. All process
// operations are serialized; Disconnect cancels an in-flight recovery first.
type DesktopLifecycle struct {
	StartupReady             func()
	StartupObservedReady     func(context.Context) bool
	StartupResume            func(context.Context, BridgeSnapshot) BridgeSnapshot
	StartupMaintenanceNeeded func(context.Context) (bool, error)
	StartupMaintenance       func(context.Context) error
	PendingInputSetup        func() string
	PendingInputRestore      func(context.Context) error
	pendingInput             *pendingInputRecovery
	updateHandedOff          bool
	ctx                      context.Context
	open                     func() (Client, error)
	ensure                   func(context.Context) error
	diagnose                 func(context.Context, string) error
	displayReady             func() bool
	diagnosticActive         atomic.Bool
	gate                     lifecycleGate
	cancelMu                 sync.Mutex
	cancel                   context.CancelFunc
	stopped                  atomic.Bool
	recovery                 AutomaticRecovery
	updatedResume            *updatedWorkspaceResume
	stateMu                  sync.RWMutex
	state                    BridgeSnapshot
}

func NewDesktopLifecycle(ctx context.Context, open func() (Client, error), ensure func(context.Context) error, diagnose func(context.Context, string) error, displayReady ...func() bool) *DesktopLifecycle {
	b := &DesktopLifecycle{ctx: ctx, open: open, ensure: ensure, diagnose: diagnose, state: bridgeFailure("preparing")}
	if len(displayReady) > 0 {
		b.displayReady = displayReady[0]
	}
	return b
}

func (b *DesktopLifecycle) Refresh() BridgeSnapshot {
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	value := b.state
	value.Peers = append([]BridgePeer{}, value.Peers...)
	return value
}

func (b *DesktopLifecycle) save(value BridgeSnapshot) BridgeSnapshot {
	// The input, returned snapshot and stored state each own their peer slice.
	// BridgePeer contains only strings, so copying its elements is sufficient.
	value.Peers = slices.Clone(value.Peers)
	b.stateMu.Lock()
	// Native projection revision, independent of HTTP response arrival order.
	if value.InputRefreshCode == "" && value.ReconnectCode == "" {
		value.InputRefreshCode, value.ReconnectCode = b.state.InputRefreshCode, b.state.ReconnectCode
	}
	value.Revision = b.state.Revision + 1
	b.state = value
	b.state.Peers = slices.Clone(value.Peers)
	b.stateMu.Unlock()
	return value
}

func (b *DesktopLifecycle) clearMaintenanceOutcome() {
	b.stateMu.Lock()
	b.state.InputRefreshCode, b.state.ReconnectCode = "", ""
	b.stateMu.Unlock()
}

func (b *DesktopLifecycle) operation() (context.Context, func()) {
	return b.operationBudget(40 * time.Second)
}

func (b *DesktopLifecycle) operationBudget(budget time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(b.ctx, budget)
	b.cancelMu.Lock()
	b.cancel = cancel
	b.cancelMu.Unlock()
	return ctx, func() {
		cancel()
		b.cancelMu.Lock()
		b.cancel = nil
		b.cancelMu.Unlock()
	}
}

func (b *DesktopLifecycle) startup(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.ensure == nil {
		return ErrAgentNotReady
	}
	if err := b.ensure(ctx); err != nil {
		return err
	}
	if b.StartupReady != nil && ctx.Err() == nil {
		b.StartupReady()
	}
	return ctx.Err()
}

func (b *DesktopLifecycle) failedStartup(ctx context.Context, err error) BridgeSnapshot {
	if pendingInputAgent(err) && b.ctx.Err() == nil && !b.stopped.Load() && !errors.Is(ctx.Err(), context.Canceled) {
		b.pendingInput = &pendingInputRecovery{startup: true, deadline: time.Now().Add(5 * time.Minute)}
	}
	if !transientUpdateStartup(err) && !pendingInputAgent(err) {
		b.updatedResume = nil
	}
	value := NewDesktopBridge(ctx, b.open).Refresh()
	value.Code = "startup_failed"
	if pendingInputAgent(err) {
		value.Code, value.InputRefreshCode, value.ReconnectCode = "input_refresh_pending", "input_refresh_pending", "not_attempted"
	}
	switch {
	case errors.Is(err, ErrAgentBusy):
		value.Code = "agent_busy"
	case errors.Is(err, ErrStartupBusy):
		value.Code = "startup_busy"
	case errors.Is(err, ErrAgentIncompatible):
		value.Code = "agent_incompatible"
	case errors.Is(err, ErrAgentRecoveryFailed):
		value.Code = "recovery_required"
	}
	return b.save(value)
}

func (b *DesktopLifecycle) Connect(alias, fingerprint string) BridgeSnapshot {
	if !validBridgePeer(alias, fingerprint) {
		return bridgeFailure("invalid_peer")
	}
	if !b.gate.TryLock() {
		return bridgeFailure("busy")
	}
	defer b.gate.Unlock()
	if b.updateHandedOff {
		return bridgeFailure("agent_busy")
	}
	ctx, done := b.operationBudget(3 * time.Minute)
	defer done()
	b.stopped.Store(false)
	b.pendingInput = nil
	b.updatedResume = nil
	b.recovery.Reset()
	b.clearMaintenanceOutcome()
	if err := b.startup(ctx); err != nil {
		value := b.failedStartup(ctx, err)
		if pendingInputAgent(err) && b.pendingInput != nil {
			b.pendingInput.alias, b.pendingInput.fingerprint = alias, fingerprint
			b.pendingInput.startup = false // Never substitute a saved login peer.
		}
		return value
	}
	return b.save(NewDesktopBridge(ctx, b.open).Connect(alias, fingerprint))
}

func (b *DesktopLifecycle) Disconnect() BridgeSnapshot {
	b.stopped.Store(true)
	b.cancelMu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.cancelMu.Unlock()
	b.gate.Lock()
	defer b.gate.Unlock()
	b.updatedResume = nil
	b.recovery.Stop()
	b.pendingInput = nil
	b.clearMaintenanceOutcome()
	ctx, done := b.operation()
	defer done()
	return b.save(NewDesktopBridge(ctx, b.open).Disconnect())
}

func (b *DesktopLifecycle) Restart() BridgeSnapshot {
	if !b.gate.TryLock() {
		return bridgeFailure("busy")
	}
	defer b.gate.Unlock()
	if b.updateHandedOff {
		return bridgeFailure("agent_busy")
	}
	ctx, done := b.operationBudget(3 * time.Minute)
	defer done()
	view := NewDesktopBridge(ctx, b.open).Refresh()
	// A restart button is never authority to interrupt an active session.
	if view.State != "FAULTED" && view.State != "UNKNOWN" && view.State != "LOCAL" {
		view.Code = "agent_busy"
		return b.save(view)
	}
	var diagnosticErr error
	if view.State == "FAULTED" {
		diagnosticErr = b.diagnoseBounded(ctx, b.recovery.Fingerprint)
	}
	b.recovery.Reset()
	b.updatedResume = nil
	b.pendingInput = nil
	b.stopped.Store(false)
	b.clearMaintenanceOutcome()
	if err := b.startup(ctx); err != nil {
		return b.failedStartup(ctx, err)
	}
	return b.save(withDiagnosticNotice(NewDesktopBridge(ctx, b.open).Refresh(), diagnosticErr))
}

// Run belongs to the native service lifetime; callers cancel ctx and join it.
// It starts once and remains active even when WebView timers are throttled.
func (b *DesktopLifecycle) Run() {
	b.gate.Lock()
	ctx, done := b.operationBudget(3 * time.Minute)
	initial := NewDesktopBridge(ctx, b.open).Refresh()
	var diagnosticErr error
	if initial.State == "FAULTED" {
		diagnosticErr = b.diagnoseBounded(ctx, "")
	}
	value := b.initialize(ctx, initial)
	b.startUpdatedResumeWindow(time.Now())
	b.save(withDiagnosticNotice(value, diagnosticErr))
	done()
	b.gate.Unlock()
	timer := time.NewTicker(1500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case now := <-timer.C:
			b.tick(now)
		}
	}
}

func (b *DesktopLifecycle) tick(now time.Time) {
	if !b.gate.TryLock() {
		return
	}
	defer b.gate.Unlock()
	ctx, done := b.operation()
	defer done()
	bridge := NewDesktopBridge(ctx, b.open)
	value := bridge.Refresh()
	if next, handled := b.continuePendingInput(ctx, now, value); handled {
		b.save(next)
		return
	}
	observationKnown := value.Code == "ok" || value.Code == "recovery_required"
	previous := b.Refresh()
	if previous.State == value.State {
		switch previous.Code {
		case "startup_failed", "startup_busy", "agent_busy", "agent_incompatible":
			if value.Code == "ok" && b.StartupObservedReady != nil && b.StartupObservedReady(ctx) && ctx.Err() == nil {
				if b.StartupReady != nil {
					b.StartupReady()
				}
			} else {
				value.Code = previous.Code
			}
		case "recovery_exhausted":
			if value.Code != "ok" || (value.State != "CONNECTED" && value.State != "CONNECTING" && value.State != "RECONNECTING") {
				value.Code = previous.Code
			}
		case "diagnostic_failed", "display_recovery_expired", "startup_resume_settings_invalid", "startup_resume_settings_unavailable":
			value.Code = previous.Code
		}
	}
	b.save(value)
	if b.stopped.Load() {
		return
	}
	if next, handled := b.resumeUpdatedConnection(ctx, now, value); handled {
		b.save(next)
		return
	}
	// Missing trust is an unavailable observation, not a removed identity or
	// permission to prepare another agent. Keep the recovery budget unchanged.
	if !observationKnown {
		if b.recovery.uncertain {
			value.Code = "outcome_unknown"
			b.save(value)
		}
		return
	}
	view := View{State: value.State, PeerAlias: value.PeerAlias, StopReason: value.StopReason, DisplayReady: true, Fingerprints: map[string]string{}}
	if value.State == "LOCAL" && value.StopReason == "display_change" && b.displayReady != nil {
		view.DisplayReady = b.displayReady()
	}
	for _, peer := range value.Peers {
		view.Fingerprints[peer.Alias] = peer.Fingerprint
	}
	action := b.recovery.Next(now, view)
	if action == "" {
		if value.State == "FAULTED" && b.recovery.Attempts >= 3 {
			value.Code = "recovery_exhausted"
			b.save(value)
		}
		return
	}
	if action == "display_recovery_expired" || action == "outcome_unknown" || action == "stale_peer" {
		value.Code = action
		b.save(value)
		return
	}
	if action == "display_reconnect" {
		value.Code = "display_reconnecting"
		b.save(value)
		if err := b.startup(ctx); err != nil {
			b.failedStartup(ctx, err)
			return
		}
		if !b.stopped.Load() && ctx.Err() == nil {
			b.save(b.recoveryResume(bridge, b.recovery.Alias, b.recovery.Fingerprint))
		}
		return
	}
	diagnosticErr := b.diagnoseBounded(ctx, b.recovery.Fingerprint)
	if diagnosticErr != nil {
		value.Code = "diagnostic_failed"
		b.save(value)
	}
	if action != "auto_recover" {
		return
	}
	value.Code = "recovering"
	b.save(value)
	if err := b.startup(ctx); err != nil {
		b.failedStartup(ctx, err)
		return
	}
	if b.stopped.Load() || ctx.Err() != nil {
		return
	}
	// DesktopBridge rechecks active trust and the exact saved fingerprint.
	b.save(withDiagnosticNotice(b.recoveryResume(bridge, b.recovery.Alias, b.recovery.Fingerprint), diagnosticErr))
}

// Agent-side arbitration also covers CLI disconnects during native preparation.
func (b *DesktopLifecycle) recoveryResume(bridge *DesktopBridge, alias, fingerprint string) BridgeSnapshot {
	value := bridge.Resume(alias, fingerprint)
	if value.Code == "cancelled" {
		b.recovery.Stop()
	}
	if value.Code == "outcome_unknown" {
		b.recovery.Alias, b.recovery.Fingerprint = alias, fingerprint
		b.recovery.uncertain = true
	}
	return value
}
