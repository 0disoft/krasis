//go:build windows

package residentagent

import (
	"context"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/wininput"
)

// The medium-integrity agent cannot capture local devices or read the cursor on
// Winlogon. When its independently verified installed backend is selected, keep
// the authenticated receiver alive without pretending to capture that desktop.
// Only the SYSTEM/UIAccess worker chooses and validates the actual input target.
type protectedSource struct {
	*wininput.SourceHook
	blocked func() bool
	event   func(string)
	native  agent.SourceHook
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
	if !h.blocked() {
		err := h.native.RunWithBoundary(ctx, session, boundary, ready)
		// Capture startup can race the lock-to-PIN switch. Native Run has
		// already joined cleanup before returning this pre-hook snapshot error.
		if wininput.SourceFailureStage(err) != "pointer-snapshot" || !h.blocked() {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	h.event("installed_source_receive_only")
	ready()
	<-ctx.Done()
	return ctx.Err()
}

func (h *protectedSource) SourceCapture() wininput.SourceActivation {
	return &protectedSourceCapture{SourceActivation: h.SourceHook.SourceCapture(), blocked: h.blocked}
}

func (h *protectedSource) PrepareReturnCursor() func() {
	if h.blocked() {
		return nil
	}
	return h.SourceHook.PrepareReturnCursor()
}

type protectedSourceCapture struct {
	wininput.SourceActivation
	blocked func() bool
}

func (c *protectedSourceCapture) Activate() error {
	if c.blocked() {
		return wininput.ErrProtectedInput
	}
	return c.SourceActivation.Activate()
}

func (c *protectedSourceCapture) ActivateProfile(ctx context.Context) error {
	if c.blocked() {
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
