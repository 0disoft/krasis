package handoff

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestWorkerApplyProbesStayBoundedAndMatching(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, ctx, out := collected(t)
		p := Begin(ctx, "receiver", "s", "e", time.Now())
		Backend(p, "installed")
		if WantWorkerApply(nil) || WantWorkerApply(context.Background()) || WantWorkerApply(p) {
			t.Fatal("untracked detail requested")
		}
		for i := 0; i < WorkerApplyProbeLimit+1; i++ {
			x, complete := Input(ctx, "receiver")
			if WantWorkerApply(x) != (i < 2) || WantWorkerApply(x) {
				t.Fatal("sample or session budget escaped")
			}
			detail := WorkerApplyProfile{Micros: [2]uint32{31001, 0}, Measured: 3}
			RecordWorkerApply(x, &detail)
			detail.Micros[0] = 999999     // stored copy must not alias the worker ACK
			RecordWorkerApply(x, &detail) // recovery cannot overwrite the original
			complete(nil)
		}
		Finish(p, "ok")
		if (*out)[0].First.WorkerApply == nil || (*out)[0].First.WorkerApply.Micros[0] != 31001 || !(*out)[0].Valid() {
			t.Fatal("matching first detail lost")
		}
		c.mu.Lock()
		c.workerApplyProbes = WorkerApplyProbeLimit
		c.mu.Unlock()
		p = Begin(ctx, "receiver", "s", "e2", time.Now())
		x, complete := Input(ctx, "receiver")
		if WantWorkerApply(x) {
			t.Fatal("reentry reset budget")
		}
		complete(nil)
		Finish(p, "ok")
		c.expire()
		if WantWorkerApply(x) {
			t.Fatal("expired window requested detail")
		}
	})
}

func TestWorkerApplyProbesSurviveHighRateInputAndReentry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, ctx, _ := collected(t)
		p := Begin(ctx, "receiver", "s", "e", time.Now())
		Backend(p, "installed")
		probes := 0
		for i := 0; i <= 3001; i++ {
			x, complete := Input(ctx, "receiver")
			requested := WantWorkerApply(x)
			if i == 2501 && !requested {
				t.Fatal("early input consumed late response diagnostics")
			}
			if requested {
				probes++
			}
			complete(nil)
			time.Sleep(time.Millisecond)
		}
		if probes != 32 || c.workerApplyProbes != uint32(probes) {
			t.Fatal("high-rate input escaped periodic probe budget", probes)
		}
		Finish(p, "ok")
		p = Begin(ctx, "receiver", "s", "e2", time.Now())
		Backend(p, "installed")
		x, complete := Input(ctx, "receiver")
		if !WantWorkerApply(x) {
			t.Fatal("periodic spacing discarded the next entry's first probe")
		}
		complete(nil)
		Finish(p, "ok")
		c.expire()
	})
}

func TestWorkerApplyPresenceSaturationAndInvalidNumbers(t *testing.T) {
	var p WorkerApplyProfile
	p.Record(0, 0)
	p.Record(1, Window+time.Second)
	p.Record(0, time.Second)
	if !p.Valid() || p.Measured != 3 || p.Saturated != 2 || p.Micros[0] != 0 {
		t.Fatal("presence or bounded saturation lost")
	}
	for _, bad := range []WorkerApplyProfile{
		{Micros: [2]uint32{1, 0}}, {Measured: 4}, {Saturated: 1},
		{Measured: 1, Saturated: 1}, {Micros: [2]uint32{uint32(Window.Microseconds()) + 1, 0}, Measured: 1},
	} {
		if bad.Valid() {
			t.Fatal("invalid detail accepted", bad)
		}
	}
}

func TestWorkerApplyPartsFitContainingSpanWhenMeasured(t *testing.T) {
	p := WorkerApplyProfile{Micros: [2]uint32{31000, 2000}, Measured: 3}
	if !p.ValidForWorker([3]uint16{0, 0, 33}, 4) || p.ValidForWorker([3]uint16{0, 0, 31}, 4) {
		t.Fatal("disjoint parts violate parent span")
	}
	if !p.ValidForWorker([3]uint16{}, 0) || !p.ValidForWorker([3]uint16{0, 0, MaxWorkerMillis}, 4) {
		t.Fatal("missing/saturated parent invented a bound")
	}
}
