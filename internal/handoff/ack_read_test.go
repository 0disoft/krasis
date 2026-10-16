package handoff

import (
	"testing"
	"time"
)

func TestACKReadValidationRequiresObservedStagesAndParentBound(t *testing.T) {
	for _, p := range []ACKReadProfile{
		{Measured: 1, Progress: ACKReadNoData},
		{Measured: 3, Progress: ACKReadData},
		{Measured: 7, Progress: ACKReadHeader},
		{Measured: 7, Progress: ACKReadComplete},
		{Measured: 7, Progress: ACKReadMatched},
	} {
		if !p.Valid() {
			t.Fatal("valid zero spans rejected", p)
		}
	}
	for _, p := range []ACKReadProfile{
		{}, {Measured: 8}, {Measured: 7, Progress: 5}, {Measured: 3, Progress: ACKReadMatched},
		{Measured: 1, Micros: [3]uint32{0, 1, 0}}, {Measured: 1, Saturated: 2},
		{Measured: 1, Saturated: 1}, {Measured: 1, Micros: [3]uint32{uint32(Window.Microseconds()) + 1, 0, 0}},
	} {
		if p.Valid() {
			t.Fatal("invalid read metadata accepted", p)
		}
	}
	p := ACKReadProfile{Progress: ACKReadComplete}
	p.Record(0, 2*time.Millisecond)
	p.Record(1, time.Millisecond)
	p.Record(2, time.Millisecond)
	v := Vector{Measured: 1 << ClientResponse}
	v.Micros[ClientResponse] = 3999
	if p.validForResponse(v) {
		t.Fatal("read spans exceeded containing response")
	}
	v.Micros[ClientResponse] = 4000
	if !p.validForResponse(v) {
		t.Fatal("disjoint spans rejected")
	}
	p.Record(0, time.Hour)
	if p.Micros[0] != 2000 {
		t.Fatal("later timing overwrote original")
	}
	var saturated ACKReadProfile
	saturated.Record(0, Window+time.Second)
	if !saturated.Valid() || saturated.Saturated != 1 {
		t.Fatal("saturation/presence lost")
	}
}

func TestACKReadSharesProbeAndCannotOverwriteOrLeakToFirstInput(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "s", "e", time.Now())
	Backend(p, "installed")
	x, done := Entry(p)
	Record(x, ClientResponse, 2*time.Second)
	profile := ACKReadProfile{Micros: [3]uint32{1935000, 0, 1000}, Measured: 7, Progress: ACKReadHeader}
	RecordACKRead(x, &profile)
	if x.Value(sampleKey{}).(*sample).vector.ACKRead != nil {
		t.Fatal("unselected sample recorded read metadata")
	}
	if !WantWorkerApply(x) {
		t.Fatal("entry probe missing")
	}
	RecordACKRead(x, &profile)
	profile.Micros[0] = 1000
	RecordACKRead(x, &profile)
	done(nil)
	i, finish := Input(ctx, "receiver")
	finish(nil)
	RecordACKRead(i, &profile)
	Finish(p, "ok")
	s := (*out)[0]
	if !s.Valid() || s.Entry == nil || s.Entry.ACKRead == nil || s.Entry.ACKRead.Micros[0] != 1935000 || s.First.ACKRead != nil || s.Frames != 0 || c.workerApplyProbes != 1 {
		t.Fatal("read timing/probe escaped its original sample", s)
	}
}
