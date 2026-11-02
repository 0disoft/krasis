package residentagent

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/productlog"
)

func TestGCObservationThresholdAndUnknownHistograms(t *testing.T) {
	for _, test := range []struct {
		w    productlog.GCWindow
		want string
	}{
		{productlog.GCWindow{LateMicros: 99999}, ""},
		{productlog.GCWindow{LateMicros: 100000}, "timer_late"},
		{productlog.GCWindow{LateMicros: 100001}, "timer_late"},
		{productlog.GCWindow{ReadMicros: 100000}, "metrics_read_slow"},
		{productlog.GCWindow{GCPauses: [6]uint64{0, 0, 0, 0, 1}}, ""},
		{productlog.GCWindow{Known: productlog.GCTotalKnown, GCPauses: [6]uint64{0, 0, 0, 1}}, ""},
		{productlog.GCWindow{Known: productlog.GCTotalKnown, GCPauses: [6]uint64{0, 0, 0, 0, 1}}, "gc_pause_bucket"},
		{productlog.GCWindow{Known: productlog.GCStoppingKnown, GCStopping: [6]uint64{0, 0, 0, 0, 0, 1}}, "gc_stopping_bucket"},
		{productlog.GCWindow{Known: productlog.GCSchedulerKnown, Runnable: [6]uint64{0, 0, 0, 0, 1}}, "runnable_bucket"},
	} {
		if got := gcObservationReason(test.w); got != test.want {
			t.Errorf("reason=%q, want %q for %+v", got, test.want, test.w)
		}
	}
}

func TestGCObservationBudgetAndCooldownPreserveFailureBudget(t *testing.T) {
	var l gcObservationLimit
	start := time.Unix(1000, 0)
	if !l.admit(start) || l.admit(start.Add(gcObservationCooldown-time.Nanosecond)) || l.admit(start.Add(-time.Second)) {
		t.Fatal("first observation or cooldown boundary incorrect")
	}
	for i := 1; i < gcObservationBudget; i++ {
		if !l.admit(start.Add(time.Duration(i) * gcObservationCooldown)) {
			t.Fatal("exact cooldown rejected", i)
		}
	}
	if l.admit(start.Add(24*time.Hour)) || l.count != gcObservationBudget || l.skipped != 3 {
		t.Fatal("observation budget is not bounded", l)
	}
	d := newRuntimeDiagnostics()
	d.ObserveFailure(start, "fault_source_hook_failed")
	if len(d.gcFailures) != 1 || d.gcSkipped.Load() != 0 {
		t.Fatal("nonfatal budget consumed failure evidence")
	}
}
