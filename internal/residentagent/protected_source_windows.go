//go:build windows

package residentagent

import (
	"context"
	"errors"
	"sync"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

// The medium-integrity agent cannot capture local devices or read the cursor on
// Winlogon. When its independently verified installed backend is selected, keep
// the authenticated receiver alive without pretending to capture that desktop.
// Only the SYSTEM/UIAccess worker chooses and validates the actual input target.
type protectedSource struct {
	*wininput.SourceHook
	blocked  func() bool
	event    func(string)
	native   agent.SourceHook
	mu       sync.Mutex
	session  *wininput.SourceSession
	stop     context.CancelCauseFunc
	refresh  bool
	prepared bool
	epoch    uint64
}

func protectedSessionHook(hook *wininput.SourceHook, event func(string)) agent.SourceHook {
	return &protectedSource{SourceHook: hook, native: hook, event: event, blocked: func() bool {
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
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.session, h.stop, h.prepared = nil, nil, false
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
		if h.blocked() {
			if !standby {
				h.observe("installed_source_receive_only")
				standby = true
			}
			initialReady.Do(ready) // Receiver/liveness readiness, not local capture.
			if err := agent.Wait(ctx, protectedRecoveryDelay); err != nil {
				return err
			}
			continue
		}
		standby = false
		attempt, stop := context.WithCancelCause(ctx)
		h.mu.Lock()
		epoch := h.epoch
		h.stop, h.refresh = stop, false
		h.mu.Unlock()
		err := h.native.RunWithBoundary(attempt, session, boundary, func() {
			h.mu.Lock()
			current := ctx.Err() == nil && attempt.Err() == nil && h.epoch == epoch && !h.refresh
			if current {
				session.ResumeCapture()
				h.prepared = true
			}
			h.mu.Unlock()
			if current {
				h.observe("installed_source_capture_ready")
				initialReady.Do(ready)
			}
		})
		stop(context.Canceled)
		h.mu.Lock()
		refresh := h.refresh
		h.stop, h.prepared = nil, false
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
		if !refresh || !errors.Is(err, context.Canceled) || !cancellationOnly(err, context.Canceled) {
			if refresh {
				h.observe("installed_source_refresh_failed")
			}
			return err
		}
		if session.PhysicalPressCount() != 0 {
			// A local press raced owner shutdown. Rebuild the full session using
			// the native guard; do not erase or carry an unmatched physical press.
			return wininput.ErrProtectedInput
		}
		h.observe("installed_source_refresh_joined")
		if err := agent.Wait(ctx, protectedRecoveryDelay); err != nil {
			return err
		}
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
	if h.SourceHook != nil && h.SourceHook.SourceCapture().Active() {
		return false
	}
	if h.session == nil || !h.session.PauseCapture() {
		return false
	}
	h.epoch++
	h.refresh, h.prepared = true, false
	if h.stop != nil {
		h.stop(wininput.ErrSourceCaptureRefresh)
	}
	return true
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
	if !h.captureReady() || h.blocked() {
		return 0, peer.ErrBoundaryInspectionUnavailable
	}
	return h.SourceHook.AdmitBoundary(ctx, intent)
}

func (h *protectedSource) SourceCapture() wininput.SourceActivation {
	return &protectedSourceCapture{SourceActivation: h.SourceHook.SourceCapture(), blocked: h.blocked, ready: h.captureReady}
}

func (h *protectedSource) PrepareReturnCursor() func() {
	if !h.captureReady() || h.blocked() {
		return nil
	}
	return h.SourceHook.PrepareReturnCursor()
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
