package handoff

import (
	"testing"
	"time"
)

func TestPreparationWaitBoundsAndPresence(t *testing.T) {
	for _, elapsed := range []time.Duration{-time.Second, 0, time.Millisecond, Window + time.Second} {
		p := StartupProfile{Schema: 1, Measured: 3, PreparationDecision: PreparationNoCandidate}
		p.RecordPreparationWait(elapsed)
		want := max(time.Duration(0), min(elapsed, Window))
		if !p.Valid() || !p.PreparationWaitMeasured || p.PreparationWaitMicros != uint32(want.Microseconds()) {
			t.Fatal("join wait lost bounds or measured-zero presence", p)
		}
	}
	for _, p := range []StartupProfile{
		{Schema: 1, Measured: 3, PreparationWaitMicros: 1},
		{Schema: 1, Measured: 3, PreparationWaitMeasured: true},
		{Schema: 1, Measured: 3, PreparationDecision: PreparationNoCandidate, PreparationWaitMeasured: true, PreparationWaitMicros: uint32(Window.Microseconds()) + 1},
	} {
		if p.Valid() {
			t.Fatal("invalid wait diagnostic accepted", p)
		}
	}
}
