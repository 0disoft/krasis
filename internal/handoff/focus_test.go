package handoff

import (
	"context"
	"testing"
	"time"
)

func TestFocusPreservesFirstCleanupAndFreezesOriginalAttempt(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "source", "first", "epoch", time.Now())
	RecordFocusState(p, FocusBeforeShow, 3)
	RecordFocusState(p, FocusAfterHide, 1)
	RecordFocusState(p, FocusAfterHide, 4)
	RecordFocusResult(p, 0, 2)
	RecordFocusResult(p, 1, 2)
	RecordFocusResult(p, 0, 3)
	stop := FocusSpan(p, 0)
	stop()
	emit := Retire(p, "ok")
	before := *getAttempt(p).summary.Focus
	next := Begin(ctx, "source", "next", "epoch", time.Now())
	RecordFocusState(p, FocusBeforeRestore, 2)
	stop()
	RecordFocusState(next, FocusBeforeShow, 1)
	emit()
	if FocusEnabled(p) || !FocusEnabled(next) || *(*out)[0].Focus != before || before.Observed != 9 || before.Restore != 2 || before.PostHide != 2 {
		t.Fatal("retired cleanup changed or attached to next attempt", *out)
	}
	// Even an internal alias cannot mutate the emitter's frozen copy.
	getAttempt(p).summary.Focus.States[0] = 4
	if *(*out)[0].Focus != before {
		t.Fatal("emitted focus was not copied")
	}
	Finish(next, "ok")
	if (*out)[1].Focus.Observed != 1 || (*out)[1].Focus.States[0] != 1 {
		t.Fatal("late cleanup polluted next attempt")
	}
}

func TestFocusDropsMissingReceiverExpiredAndExhaustedContexts(t *testing.T) {
	RecordFocusState(nil, FocusBeforeShow, 1)
	FocusSpan(context.Background(), 0)()
	c, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "r", "e", time.Now())
	RecordFocusResult(p, 0, 2)
	if FocusEnabled(p) {
		t.Fatal("receiver enabled source queries")
	}
	Finish(p, "ok")
	if (*out)[0].Focus != nil {
		t.Fatal("receiver accepted focus profile")
	}
	p = Begin(ctx, "source", "s", "e", time.Now())
	stop := FocusSpan(p, 0)
	c.started = time.Now().Add(-Window)
	stop()
	RecordFocusState(p, FocusBeforeShow, 1)
	if FocusEnabled(p) {
		t.Fatal("expired attempt enabled extra queries")
	}
	Finish(p, "ok")
	if (*out)[1].Focus != nil {
		t.Fatal("expired attempt recorded focus")
	}
	c.started = time.Now()
	c.count = Limit
	p = Begin(ctx, "source", "untracked", "e", time.Now())
	if FocusEnabled(p) {
		t.Fatal("exhausted quota enabled extra queries")
	}
}

func TestFocusValidationRejectsInvalidMasksAndUnobservedValues(t *testing.T) {
	good := FocusProfile{Schema: 1, States: [4]uint8{3, 2, 3, 1}, Observed: 15, Restore: 2, PostHide: 2, ResultMeasured: 3, Measured: 3}
	if !good.Valid() {
		t.Fatal("valid profile rejected")
	}
	for _, mutate := range []func(*FocusProfile){
		func(p *FocusProfile) { p.Schema = 2 },
		func(p *FocusProfile) { p.Observed = 16 },
		func(p *FocusProfile) { p.States[0] = 0 },
		func(p *FocusProfile) { p.States[0] = 5 },
		func(p *FocusProfile) { p.ResultMeasured = 4 },
		func(p *FocusProfile) { p.Restore = 5 },
		func(p *FocusProfile) { p.PostHide = 4 },
		func(p *FocusProfile) { p.Measured = 4 },
		func(p *FocusProfile) { p.Micros[0] = uint32(Window.Microseconds() + 1) },
		func(p *FocusProfile) { p.Measured = 0; p.Micros[0] = 1 },
	} {
		bad := good
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("invalid profile accepted", bad)
		}
	}
	if (FocusProfile{Schema: 1}).Valid() {
		t.Fatal("empty detail accepted")
	}
}
