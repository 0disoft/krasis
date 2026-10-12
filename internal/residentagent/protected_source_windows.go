//go:build windows

package residentagent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
	"github.com/0disoft/krasis/internal/winlifecycle"
)

// The medium-integrity agent cannot capture local devices or read the cursor on
// Winlogon. When its independently verified installed backend is selected, keep
// the authenticated receiver alive without pretending to capture that desktop.
// Only the SYSTEM/UIAccess worker chooses and validates the actual input target.
type protectedSource struct {
	*wininput.SourceHook
	blocked         func() bool
	cursorProtected func() bool
	sessionLocked   func() bool
	event           func(string)
	native          agent.SourceHook
	secure          agent.SourceHook
	owner           agent.SourceHook
	remoteOwner     bool
	mu              sync.Mutex
	session         *wininput.SourceSession
	stop            context.CancelCauseFunc
	refresh         bool
	prepared        bool
	epoch           uint64
	wake            chan struct{}
	protectedCursor peer.ReturnCursorPresentation
}

func protectedSessionHook(hook *wininput.SourceHook, event func(string)) agent.SourceHook {
	return &protectedSource{SourceHook: hook, native: hook, event: event, sessionLocked: winlifecycle.SessionLocked, cursorProtected: wininput.ProtectedReturnCursorRequired, blocked: func() bool {
		switch wininput.InspectProtectedInput() {
		case wininput.ProtectedInputDesktopUnavailable, wininput.ProtectedInputNonDefaultDesktop:
			return true
		default:
			return false
		}
	}}
}

func (h *protectedSource) RunWithBoundary(ctx context.Context, session *wininput.SourceSession, boundary wininput.BoundaryObserver, ready func()) error {
	if ctx == nil || session == nil || ready == nil {
		return wininput.ErrProtectedInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !session.PauseCapture() {
		return wininput.ErrProtectedInput
	}
	h.mu.Lock()
	h.session, h.prepared, h.refresh = session, false, false
	h.wake = make(chan struct{}, 1)
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.session, h.stop, h.prepared, h.owner = nil, nil, false, nil
		h.mu.Unlock()
	}()
	unregister := registerSourceRefresh(ctx, h.refreshDesktop)
	defer unregister()
	var initialReady sync.Once
	standby := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		protected := h.blocked() || h.sessionLocked != nil && h.sessionLocked() || h.cursorProtected != nil && h.cursorProtected()
		selected := h.native
		usingSecure := protected && h.secure != nil
		if usingSecure {
			selected = h.secure
		}
		if h.blocked() && !usingSecure {
			if !standby {
				h.observe("installed_source_receive_only")
				standby = true
			}
			initialReady.Do(ready) // Receiver/liveness readiness, not local capture.
			if err := h.waitDesktop(ctx); err != nil {
				return err
			}
			continue
		}
		standby = false
		attempt, stop := context.WithCancelCause(ctx)
		h.mu.Lock()
		epoch := h.epoch
		h.owner = selected
		h.remoteOwner = usingSecure
		h.stop, h.refresh = stop, false
		h.mu.Unlock()
		if usingSecure && !session.UseExternalPhysicalCapture() {
			stop(context.Canceled)
			return wininput.ErrProtectedInput
		}
		err := selected.RunWithBoundary(attempt, session, boundary, func() {
			h.mu.Lock()
			current := ctx.Err() == nil && attempt.Err() == nil && h.epoch == epoch && !h.refresh
			if current {
				session.ResumeCapture()
				h.prepared = true
			}
			h.mu.Unlock()
			if current {
				code := "installed_source_capture_ready"
				if usingSecure {
					code = "installed_source_capture_protected_ready"
				}
				h.observe(code)
				initialReady.Do(ready)
			}
		})
		stop(context.Canceled)
		h.mu.Lock()
		refresh := h.refresh
		h.stop, h.prepared, h.owner = nil, false, nil
		h.mu.Unlock()
		if ctx.Err() != nil {
			return err
		}
		if wininput.SourcePointerSnapshotFailureOnly(err) && h.blocked() {
			if err := agent.Wait(ctx, protectedRecoveryDelay); err != nil {
				return err
			}
			continue // Failed before native owners existed; preserve receive-only.
		}
		// Native Run must join every hook/window owner before returning. Only
		// our idle-owner cancellation may continue; cleanup failures stay fatal.
		cleanRetirement := usingSecure && inputworker.CleanSourceRetirement(err)
		if cleanRetirement && session.Active() {
			return wininput.ErrProtectedInput
		}
		retired := cleanRetirement && !session.Active()
		if (!refresh || !errors.Is(err, context.Canceled) || !cancellationOnly(err, context.Canceled)) && !retired {
			if refresh {
				h.observe("installed_source_refresh_failed")
			}
			return err
		}
		if retired && !session.PauseCapture() {
			return wininput.ErrProtectedInput
		}
		if session.PhysicalPressCount() != 0 {
			// A local press raced owner shutdown. Rebuild the full session using
			// the native guard; do not erase or carry an unmatched physical press.
			return wininput.ErrProtectedInput
		}
		h.observe("installed_source_refresh_joined")
		// Reinspect immediately after the old owners have joined. A secure
		// desktop still uses bounded standby polling; a ready Default desktop
		// does not need another unconditional recovery delay.
	}
}

// Called only by the verified lifecycle event owner. No log callback runs while
// holding this gate. Concurrent grant activation shares session.PauseCapture.
func (h *protectedSource) refreshDesktop(event agentlifecycle.Event) bool {
	if event != agentlifecycle.EventSessionLock && event != agentlifecycle.EventDesktopSwitch {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.owner != nil {
		if capture, ok := h.owner.(interface {
			SourceCapture() wininput.SourceActivation
		}); ok && capture.SourceCapture().Active() {
			return false
		}
	} else if h.SourceHook != nil && h.SourceHook.SourceCapture().Active() {
		return false
	}
	if h.session == nil || !h.session.PauseCapture() {
		return false
	}
	h.epoch++
	h.refresh, h.prepared = true, false
	select {
	case h.wake <- struct{}{}:
	default:
	}
	if h.stop != nil {
		h.stop(wininput.ErrSourceCaptureRefresh)
	}
	return true
}

func (h *protectedSource) waitDesktop(ctx context.Context) error {
	timer := time.NewTimer(protectedRecoveryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.wake:
		return nil
	case <-timer.C:
		return nil
	}
}

func (h *protectedSource) captureReady() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.prepared
}

func (h *protectedSource) observe(code string) {
	if h.event != nil {
		h.event(code)
	}
}

func (h *protectedSource) AdmitBoundary(ctx context.Context, intent control.BoundaryIntent) (peer.BoundaryAdmissionDecision, error) {
	if ctx == nil {
		return 0, wininput.ErrProtectedInput
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	h.mu.Lock()
	owner, prepared, remote := h.owner, h.prepared, h.remoteOwner
	h.mu.Unlock()
	if prepared && owner != nil && remote {
		return owner.(peer.BoundaryAdmissionGate).AdmitBoundary(ctx, intent)
	}
	if !prepared || h.blocked() {
		return 0, peer.ErrBoundaryInspectionUnavailable
	}
	return h.SourceHook.AdmitBoundary(ctx, intent)
}

func (h *protectedSource) SourceCapture() wininput.SourceActivation {
	return &protectedSourceCapture{SourceActivation: &selectedCapture{owner: h}, blocked: func() bool {
		h.mu.Lock()
		secure := h.owner != nil && h.remoteOwner
		h.mu.Unlock()
		return h.blocked() && !secure
	}, ready: h.captureReady}
}

func (h *protectedSource) SetProtectedSource(source agent.SourceHook) {
	if _, ok := source.(peer.BoundaryAdmissionGate); !ok {
		return
	}
	if _, ok := source.(interface {
		SourceCapture() wininput.SourceActivation
	}); !ok {
		return
	}
	h.secure = source
}

// Resolve after selection without holding the resident gate during IPC.
type selectedCapture struct{ owner *protectedSource }

func (c *selectedCapture) current() wininput.SourceActivation {
	h := c.owner
	h.mu.Lock()
	owner := h.owner
	h.mu.Unlock()
	if provider, ok := owner.(interface {
		SourceCapture() wininput.SourceActivation
	}); ok {
		return provider.SourceCapture()
	}
	if h.SourceHook != nil {
		return h.SourceHook.SourceCapture()
	}
	return nil
}
func (c *selectedCapture) Activate() error {
	if current := c.current(); current != nil {
		return current.Activate()
	}
	return wininput.ErrProtectedInput
}
func (c *selectedCapture) ActivateProfile(ctx context.Context) error {
	current := c.current()
	if profiled, ok := current.(wininput.SourceProfiledActivation); ok {
		return profiled.ActivateProfile(ctx)
	}
	if current != nil {
		return current.Activate()
	}
	return wininput.ErrProtectedInput
}
func (c *selectedCapture) DeactivateProfile(ctx context.Context) {
	current := c.current()
	if profiled, ok := current.(wininput.SourceProfiledDeactivation); ok {
		profiled.DeactivateProfile(ctx)
		return
	}
	if current != nil {
		current.Deactivate()
	}
}
func (c *selectedCapture) Deactivate() {
	if current := c.current(); current != nil {
		current.Deactivate()
	}
}
func (c *selectedCapture) Active() bool {
	current := c.current()
	return current != nil && current.Active()
}

func (h *protectedSource) PrepareReturnCursor() func() {
	h.mu.Lock()
	owner, remote, prepared := h.owner, h.remoteOwner, h.prepared
	h.mu.Unlock()
	if remote && prepared {
		if presenter, ok := owner.(peer.ReturnCursorPresentation); ok {
			h.observe("installed_return_cursor_route_capture")
			return presenter.PrepareReturnCursor()
		}
	}
	if !h.captureReady() || h.blocked() || h.cursorProtected != nil && h.cursorProtected() {
		if h.protectedCursor != nil {
			h.observe("installed_return_cursor_route_protected")
			return h.protectedCursor.PrepareReturnCursor()
		}
		return nil
	}
	h.observe("installed_return_cursor_route_native")
	return h.SourceHook.PrepareReturnCursor()
}

func (h *protectedSource) SetProtectedReturnCursor(cursor peer.ReturnCursorPresentation) {
	h.protectedCursor = cursor
}
func (h *protectedSource) RestoreReturnCursor() {
	h.SourceHook.RestoreReturnCursor()
	if presenter, ok := h.secure.(peer.ReturnCursorPresentation); ok {
		presenter.RestoreReturnCursor()
	}
	if h.protectedCursor != nil {
		h.protectedCursor.RestoreReturnCursor()
	}
}

type protectedSourceCapture struct {
	wininput.SourceActivation
	blocked func() bool
	ready   func() bool
}

func (c *protectedSourceCapture) Activate() error {
	if c.blocked() || c.ready != nil && !c.ready() {
		return wininput.ErrProtectedInput
	}
	return c.SourceActivation.Activate()
}

func (c *protectedSourceCapture) ActivateProfile(ctx context.Context) error {
	if c.blocked() || c.ready != nil && !c.ready() {
		return wininput.ErrProtectedInput
	}
	if profiled, ok := c.SourceActivation.(wininput.SourceProfiledActivation); ok {
		return profiled.ActivateProfile(ctx)
	}
	return c.SourceActivation.Activate()
}

func (c *protectedSourceCapture) DeactivateProfile(ctx context.Context) {
	if profiled, ok := c.SourceActivation.(wininput.SourceProfiledDeactivation); ok {
		profiled.DeactivateProfile(ctx)
		return
	}
	c.SourceActivation.Deactivate()
}
