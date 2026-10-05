package handoff

import (
	"testing"
	"time"
)

func TestPostHideRestoreEvidencePreservesRejectionAndRetirement(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "source", "session", "epoch", time.Now())
	RecordFocusResult(p, 0, 3)
	RecordPostHideRestore(p, 3, true, 2*time.Millisecond)
	RecordPostHideRestore(p, 1, false, time.Second)
	emit := Retire(p, "ok")
	RecordPostHideRestore(p, 4, false, time.Second)
	next := Begin(ctx, "source", "next", "epoch", time.Now())
	RecordFocusResult(next, 0, 2)
	RecordPostHideRestore(next, 3, true, time.Second)
	emit()
	Finish(next, "ok")
	f := (*out)[0].Focus
	if !(*out)[0].Valid() || !f.PostHideRestoreMeasured || f.PostHideRestoreForeground != 3 || !f.PostHideRestoreAccepted || f.PostHideRestoreMicros != 2000 || f.Restore != 3 || (*out)[1].Focus.PostHideRestoreMeasured {
		t.Fatal("post-hide evidence changed rejection or crossed attempt ownership", *out)
	}
}

func TestPostHideRestoreEvidenceValidationRejectsIncompleteOrInconsistentValues(t *testing.T) {
	good := FocusProfile{Schema: 1, Restore: 3, ResultMeasured: 1, PostHideRestoreForeground: 3, PostHideRestoreAccepted: true, PostHideRestoreMeasured: true, PostHideRestoreMicros: 2000}
	if !good.Valid() {
		t.Fatal("valid post-hide restoration rejected")
	}
	for _, mutate := range []func(*FocusProfile){
		func(p *FocusProfile) { p.Restore = 2 }, func(p *FocusProfile) { p.ResultMeasured = 0 },
		func(p *FocusProfile) { p.PostHideRestoreForeground = 0 }, func(p *FocusProfile) { p.PostHideRestoreForeground = 5 },
		func(p *FocusProfile) { p.PostHideRestoreMeasured = false }, func(p *FocusProfile) { p.PostHideRestoreMicros = uint32(Window.Microseconds()) + 1 },
	} {
		bad := good
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("invalid evidence accepted", bad)
		}
	}
	_, ctx, out := collected(t)
	p := Begin(ctx, "source", "session", "epoch", time.Now())
	RecordFocusResult(p, 0, 3)
	RecordPostHideRestore(p, 3, true, -time.Second)
	RecordPostHideRestore(p, 5, true, time.Second)
	RecordPostHideRestore(p, 1, false, Window+time.Second)
	Finish(p, "ok")
	if !(*out)[0].Valid() || (*out)[0].Focus.PostHideRestoreMicros != uint32(Window.Microseconds()) || (*out)[0].Focus.PostHideRestoreAccepted {
		t.Fatal("invalid duration or state leaked", *out)
	}
}
