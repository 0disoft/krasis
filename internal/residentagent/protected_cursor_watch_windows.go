//go:build windows

package residentagent

import (
	"context"
	"time"
)

const protectedCursorWatchInterval = 100 * time.Millisecond

// A credential broker can protect cursor presentation on Default without a
// desktop-switch notification. Promote the idle native owner once, so later
// returns reuse the authenticated capture shield instead of spawning a passive
// child each time. The same press/activation gate as lifecycle refresh applies.
// Do not demote just because foreground focus changes during a handoff.
func (h *protectedSource) watchProtectedCursor(ctx context.Context, epoch uint64, secure bool) func() {
	if secure || h.secure == nil || h.cursorProtected == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(protectedCursorWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if !h.cursorProtected() {
				continue
			}
			h.mu.Lock()
			current := ctx.Err() == nil && h.epoch == epoch && h.owner != nil && !h.remoteOwner && h.prepared && !h.refresh
			refreshed := current && h.refreshIdleOwnerLocked()
			h.mu.Unlock()
			if refreshed {
				h.observe("installed_source_cursor_promoted")
				return
			}
		}
	}()
	return func() { <-done }
}
