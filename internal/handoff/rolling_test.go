package handoff

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestRollingProfilesResumeAfterIdleAndRejectOldWindowCallbacks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var out []Summary
		c := NewRolling(func(s Summary) {
			if !s.Valid() {
				t.Error("invalid rolling summary", s)
			}
			out = append(out, s)
		})
		ctx := WithSession(context.Background(), c)
		old := Begin(ctx, "source", "old", "epoch", time.Now())
		oldWindow := c.started
		x, complete := Input(ctx, "source")
		time.Sleep(20 * time.Minute)
		if len(out) != 1 || !out[0].Incomplete {
			t.Fatal("idle window did not flush", len(out))
		}
		current := Begin(ctx, "source", "new", "epoch", time.Now())
		if getAttempt(current) == nil || getAttempt(current) == getAttempt(old) {
			t.Fatal("later handoff remained invisible")
		}
		c.expireWindow(oldWindow)
		if WantPipeOwnerCycles(x) {
			t.Fatal("late old sample consumed a new window's diagnostic budget")
		}
		Record(x, ClientResponse, time.Second)
		complete(nil)
		Record(old, CaptureActivate, time.Second)
		Record(current, CaptureActivate, time.Millisecond)
		Finish(current, "ok")
		if len(out) != 2 || out[1].Preparation.Micros[CaptureActivate] != 1000 || out[1].Frames != 0 || c.stopped.Load() {
			t.Fatal("old sample/timer contaminated the later window", out, c.stopped.Load())
		}
		c.timer.Stop()
	})
}

func TestRollingReconnectRetainsExchangeTagConfiguration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := NewRolling(func(Summary) {})
		ctx := WithSession(context.Background(), c)
		Finish(Begin(ctx, "source", "old", "e", time.Now()), "ok")
		time.Sleep(Window)
		ctx = WithSession(context.Background(), c)
		const tag = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		ConfigureExchangeTags(ctx, func(string, string) string { return tag })
		next := Begin(ctx, "source", "new", "e", time.Now())
		if getAttempt(next).summary.ExchangeTag != tag {
			t.Fatal("a reconnect between windows lost A/B correlation")
		}
		Finish(next, "ok")
		c.timer.Stop()
	})
}

func TestRollingProfilesKeepWindowAndLifetimeProbeBudgets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := NewRolling(func(Summary) {})
		ctx := WithSession(context.Background(), c)
		for i := 0; i < Limit+5; i++ {
			Finish(Begin(ctx, "source", "s", "e", time.Now()), "ok")
		}
		if c.count != Limit {
			t.Fatal("rolling reset budget on handoff", c.count)
		}
		c.pipeOwnerProbes.Store(PipeOwnerProbeLimit)
		c.workerApplyProbes = WorkerApplyProbeLimit
		c.cursorPlacementProbes = CursorPlacementProbeLimit
		time.Sleep(Window)
		fresh := Begin(ctx, "source", "new", "e", time.Now())
		if getAttempt(fresh) == nil || c.count != 1 {
			t.Fatal("new window did not admit a bounded attempt")
		}
		if c.pipeOwnerProbes.Load() != PipeOwnerProbeLimit || c.workerApplyProbes != WorkerApplyProbeLimit || c.cursorPlacementProbes != CursorPlacementProbeLimit {
			t.Fatal("rolling replenished native diagnostic probes")
		}
		Finish(fresh, "ok")
		c.timer.Stop()
	})
}
