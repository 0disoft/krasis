package productpanel

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

// Called by local maintenance after lifecycle has disconnected the
// session; startup ownership fences other UIs and verifies exit before writes.
func (s Startup) RefreshProtectedInput(ctx context.Context) error {
	return s.maintainProtectedInput(ctx, s.InputRefresh, false)
}

// A verified new app launch may replace an older agent after safe disconnect.
// BeforeStart retains automatic consent's one-attempt and pending-result policy.
func (s Startup) RefreshProtectedInputAtLaunch(ctx context.Context) error {
	return s.maintainProtectedInput(ctx, s.BeforeStart, true)
}

func (s Startup) maintainProtectedInput(ctx context.Context, refresh func(context.Context) error, allowOlder bool) error {
	if refresh == nil {
		return ErrInputRefreshFailed
	}
	err := s.refreshProtectedInput(ctx, refresh, allowOlder)
	if err != nil && !errors.Is(err, ErrInputRefreshCancelled) && !errors.Is(err, ErrInputRefreshFailed) {
		return err
	}
	// Restore an ordinary or protected agent after terminal cancellation/failure.
	// An expired consent context cannot suppress this bounded recovery.
	recoverCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 40*time.Second)
	defer cancel()
	if startErr := s.Ensure(recoverCtx); startErr != nil {
		return startErr
	}
	return err
}

func (s Startup) refreshProtectedInput(ctx context.Context, refresh func(context.Context) error, allowOlder bool) error {
	lease, err := s.Lock()
	if err != nil {
		return ErrStartupBusy
	}
	defer lease.Close()
	if err := s.Verify(); err != nil {
		return err
	}
	client, err := s.Open()
	if err == nil {
		info, err := client.Runtime(ctx)
		if err != nil {
			return err
		}
		if !s.maintenanceCompatible(info, allowOlder) {
			return ErrAgentIncompatible
		}
		result, err := client.StopForUpgrade(ctx, info.Instance, s.Generation, s.Revision)
		if err != nil {
			return err
		}
		if result.Status != "COMPLETED" || result.Code != "upgrade_stop_requested" {
			return ErrAgentBusy
		}
	} else if !errors.Is(err, localapi.ErrUnavailable) && !errors.Is(err, localapi.ErrTransport) {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		available, err := s.Available()
		if err != nil {
			return err
		}
		if available {
			return refresh(ctx)
		}
		if err := s.Pause(ctx); err != nil {
			return err
		}
	}
}

// Keep the UI and its recovery record alive. Agent exit, input release,
// maintenance and reconnect share the same gate as peer update application.
func (b *DesktopLifecycle) RefreshProtectedInput(refresh func(context.Context) error) BridgeSnapshot {
	if refresh == nil || !b.gate.TryLock() {
		return bridgeFailure("busy")
	}
	defer b.gate.Unlock()
	ctx, done := b.operationBudget(3 * time.Minute)
	defer done()
	bridge := NewDesktopBridge(ctx, b.open)
	before := bridge.Refresh()
	return b.refreshProtectedInput(ctx, before, refresh)
}

// Caller owns gate and the operation context; startup and the button share the
// same disconnect/reconnect transaction rather than nesting locks.
func (b *DesktopLifecycle) refreshProtectedInput(ctx context.Context, before BridgeSnapshot, refresh func(context.Context) error) BridgeSnapshot {
	bridge := NewDesktopBridge(ctx, b.open)
	alias, fingerprint := before.PeerAlias, ""
	for _, peer := range before.Peers {
		if peer.Alias == alias {
			fingerprint = peer.Fingerprint
		}
	}
	if before.State != "LOCAL" && before.State != "FAULTED" && before.State != "UNKNOWN" {
		stopped := bridge.Disconnect()
		if stopped.Code != "ok" || stopped.State != "LOCAL" {
			return b.save(stopped)
		}
	}
	b.recovery.Reset()
	err := refresh(ctx)
	// Even if the installer takes the whole budget, restore the saved trusted
	// identity only while the UI is alive and the user has not cancelled it.
	reconnect, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	bridge = NewDesktopBridge(reconnect, b.open)
	value := bridge.Refresh()
	if b.ctx.Err() == nil && ctx.Err() == nil && alias != "" && fingerprint != "" && !errors.Is(err, ErrInputRefreshPending) && !errors.Is(err, ErrStartupBusy) {
		value = bridge.Connect(alias, fingerprint)
	}
	if err != nil {
		switch {
		case errors.Is(err, ErrInputRefreshCancelled):
			value.Code = "input_refresh_cancelled"
		case errors.Is(err, ErrInputRefreshPending):
			value.Code = "input_refresh_pending"
		case errors.Is(err, ErrStartupBusy), errors.Is(err, ErrAgentBusy):
			value.Code = "busy"
		default:
			value.Code = "input_refresh_failed"
		}
	}
	return b.save(value)
}
