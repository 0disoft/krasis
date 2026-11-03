package productlog

import "testing"

func TestRuntimeSamplerLatenessIsIndependentAndNonnegative(t *testing.T) {
	s := RuntimeSummary{IntervalMicros: 1, DispatchKind: "idle", SamplerTimerLateMicros: 1852582}
	if !s.valid() || s.TimerLateMicros != 0 {
		t.Fatal("diagnostic-only stall rejected or attributed to liveness")
	}
	s.SamplerTimerLateMicros = -1
	if s.valid() {
		t.Fatal("negative diagnostic timer duration accepted")
	}
}
