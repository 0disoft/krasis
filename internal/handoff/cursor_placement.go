package handoff

import (
	"context"
	"time"
)

const CursorPlacementProbeLimit = 64

// One request per tracked receiver entry, inside the original collection
// window. Reentry/reconnect cannot reset the collector's total budget.
func WantCursorPlacement(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || s.collector == nil || s.collector.stopped.Load() {
		return false
	}
	c := s.collector
	c.mu.Lock()
	defer c.mu.Unlock()
	if a.cursorPlacementRequested || a.finished || a.summary.Role != "receiver" || a.summary.Backend != "installed" || c.started.IsZero() || time.Since(c.started) >= Window || c.cursorPlacementProbes >= CursorPlacementProbeLimit {
		return false
	}
	a.cursorPlacementRequested = true
	c.cursorPlacementProbes++
	return true
}
