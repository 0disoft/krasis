package handoff

import (
	"testing"
	"time"
)

func TestFocusFallbackRetainsFirstResultAndRejectsLateOrExpiredOwners(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "source", "first", "epoch", time.Now())
	RecordFocusFallback(p, 2, FocusFallbackProbeFailed)
	RecordFocusFallback(p, 1, FocusFallbackSucceeded)
	Finish(p, "error")
	before := *(*out)[0].Focus
	next := Begin(ctx, "source", "next", "epoch", time.Now())
	RecordFocusFallback(p, 4, FocusFallbackSetFailed)
	c.started = time.Now().Add(-Window)
	RecordFocusFallback(next, 3, FocusFallbackDetachFailed)
	Finish(next, "error")
	if before.RestoreFallback != 2 || before.RestoreFallbackStage != FocusFallbackProbeFailed || !before.RestoreFallbackMeasured || *(*out)[0].Focus != before || (*out)[1].Focus != nil {
		t.Fatal("fallback overwritten, moved to next attempt or recorded after expiry", *out)
	}
	RecordFocusFallback(nil, 1, FocusFallbackSucceeded)
	c.started = time.Now()
	receiver := Begin(ctx, "receiver", "receiver", "epoch", time.Now())
	RecordFocusFallback(receiver, 1, FocusFallbackSucceeded)
	Finish(receiver, "ok")
	if (*out)[2].Focus != nil {
		t.Fatal("receiver accepted source fallback")
	}
}

func TestRejectedForegroundIsFirstWriteBoundedAndFrozen(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "source", "first", "epoch", time.Now())
	RecordFocusRestoreRejectedForeground(p, 0)
	RecordFocusRestoreRejectedForeground(p, 5)
	RecordFocusRestoreRejectedForeground(p, 3)
	RecordFocusRestoreRejectedForeground(p, 2)
	RecordFocusFallback(p, 4, FocusFallbackSetFailed)
	emit := Retire(p, "error")
	before := *getAttempt(p).summary.Focus
	getAttempt(p).summary.Focus.RestoreRejectedForeground = 4
	RecordFocusRestoreRejectedForeground(p, 1)
	emit()
	if !before.Valid() || !before.RestoreRejectedForegroundMeasured || before.RestoreRejectedForeground != 3 || *(*out)[0].Focus != before {
		t.Fatal("first rejection observation overwritten or not copied", *out)
	}
	next := Begin(ctx, "source", "next", "epoch", time.Now())
	c.started = time.Now().Add(-Window)
	RecordFocusRestoreRejectedForeground(next, 1)
	Finish(next, "error")
	c.started = time.Now()
	receiver := Begin(ctx, "receiver", "receiver", "epoch", time.Now())
	RecordFocusRestoreRejectedForeground(receiver, 1)
	Finish(receiver, "ok")
	RecordFocusRestoreRejectedForeground(nil, 1)
	if (*out)[1].Focus != nil || (*out)[2].Focus != nil {
		t.Fatal("expired or foreign role accepted rejection observation", *out)
	}
}

func TestRejectedForegroundValidationRequiresPresenceAndCompatibleExit(t *testing.T) {
	good := FocusProfile{Schema: 1, RestoreFallback: 4, RestoreFallbackStage: FocusFallbackSetFailed, RestoreFallbackMeasured: true,
		RestoreRejectedForeground: 2, RestoreRejectedForegroundMeasured: true}
	for state := uint8(1); state <= 4; state++ {
		p := good
		p.RestoreRejectedForeground = state
		if !p.Valid() {
			t.Fatal("valid foreground category rejected", p)
		}
		p.RestoreFallback, p.RestoreFallbackStage = 3, FocusFallbackDetachFailed
		if !p.Valid() {
			t.Fatal("later detach error lost the earlier observation", p)
		}
	}
	for _, mutate := range []func(*FocusProfile){
		func(p *FocusProfile) { p.RestoreRejectedForegroundMeasured = false },
		func(p *FocusProfile) { p.RestoreRejectedForeground = 0 },
		func(p *FocusProfile) { p.RestoreRejectedForeground = 5 },
		func(p *FocusProfile) {
			p.RestoreFallbackMeasured = false
			p.RestoreFallback = 0
			p.RestoreFallbackStage = 0
		},
		func(p *FocusProfile) { p.RestoreFallbackStage = FocusFallbackTerminalBeforeSet },
		func(p *FocusProfile) { p.RestoreFallback = 1; p.RestoreFallbackStage = FocusFallbackSucceeded },
	} {
		bad := good
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("contradictory rejection observation accepted", bad)
		}
	}
}

func TestFocusFallbackValidationChecksPresenceAndResultStagePairs(t *testing.T) {
	for stage := FocusFallbackSucceeded; stage <= FocusFallbackVerificationLost; stage++ {
		matches := 0
		for result := uint8(1); result <= 5; result++ {
			p := FocusProfile{Schema: 1, RestoreFallback: result, RestoreFallbackStage: stage, RestoreFallbackMeasured: true}
			if p.Valid() {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("stage does not have exactly one existing result", stage, matches)
		}
	}
	good := FocusProfile{Schema: 1, RestoreFallback: 2, RestoreFallbackStage: FocusFallbackProbeFailed, RestoreFallbackMeasured: true}
	for _, mutate := range []func(*FocusProfile){
		func(p *FocusProfile) { p.RestoreFallbackMeasured = false },
		func(p *FocusProfile) { p.ResultMeasured = 8 },
		func(p *FocusProfile) { p.RestoreFallback = 0 },
		func(p *FocusProfile) { p.RestoreFallback = 6 },
		func(p *FocusProfile) { p.RestoreFallbackStage = 0 },
		func(p *FocusProfile) { p.RestoreFallbackStage = 16 },
		func(p *FocusProfile) { p.RestoreFallbackStage = FocusFallbackDetachFailed },
	} {
		bad := good
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("invalid fallback evidence accepted", bad)
		}
	}
}
