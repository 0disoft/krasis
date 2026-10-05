package handoff

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCursorPlacementProbeWindowRoleAndReentryBudget(t *testing.T) {
	c, ctx, _ := collected(t)
	if WantCursorPlacement(nil) || WantCursorPlacement(context.Background()) || WantCursorPlacement(ctx) {
		t.Fatal("untracked placement probed")
	}
	p := Begin(ctx, "source", "s", "e", time.Now())
	if WantCursorPlacement(p) {
		t.Fatal("source probed")
	}
	Finish(p, "ok")
	p = Begin(ctx, "receiver", "s", "e", time.Now())
	if WantCursorPlacement(p) {
		t.Fatal("ordinary backend probed")
	}
	Backend(p, "installed")
	c.cursorPlacementProbes = CursorPlacementProbeLimit - 1
	var accepted atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if WantCursorPlacement(p) {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if accepted.Load() != 1 || c.cursorPlacementProbes != CursorPlacementProbeLimit {
		t.Fatal("budget or attempt cap escaped")
	}
	Finish(p, "error")
	p = Begin(ctx, "receiver", "s2", "e2", time.Now())
	Backend(p, "installed")
	if WantCursorPlacement(p) {
		t.Fatal("reconnect reset probe budget")
	}
	Finish(p, "ok")
}

func TestCursorPlacementProbeExpiresAndIgnoresFinishedOrCancelledEntry(t *testing.T) {
	for _, mode := range []string{"expired", "stopped", "finished", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			c, ctx, _ := collected(t)
			p := Begin(ctx, "receiver", "s", "e", time.Now())
			Backend(p, "installed")
			switch mode {
			case "expired":
				c.started = time.Now().Add(-Window)
			case "stopped":
				c.expire()
			case "finished":
				Finish(p, "ok")
			case "cancelled":
				var cancel context.CancelFunc
				p, cancel = context.WithCancel(p)
				cancel()
			}
			if WantCursorPlacement(p) {
				t.Fatal("inactive entry probed")
			}
		})
	}
}
