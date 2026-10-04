package handoff

import (
	"context"
	"testing"
	"time"
)

func TestPipeOwnerCycleProbeBoundedAndAbsentOutsideInput(t *testing.T) {
	c, ctx, _ := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	Backend(p, "installed")
	if WantPipeOwnerCycles(nil) || WantPipeOwnerCycles(context.Background()) || WantPipeOwnerCycles(p) {
		t.Fatal("probe escaped tracked input")
	}
	for i := 0; i < PipeOwnerProbeLimit+2; i++ {
		x, complete := Input(ctx, "receiver")
		if WantPipeOwnerCycles(x) != (i < PipeOwnerProbeLimit) {
			t.Fatal("probe budget missing or reset")
		}
		RecordAuthority(x, AuthorityPipeOwner, time.Microsecond)
		if WantPipeOwnerCycles(x) {
			t.Fatal("later authority check replaced matching query")
		}
		complete(nil)
	}
	Finish(p, "ok")
	p = Begin(ctx, "receiver", "session", "epoch-2", time.Now())
	x, complete := Input(ctx, "receiver")
	if WantPipeOwnerCycles(x) {
		t.Fatal("reentry reset collector budget")
	}
	complete(nil)
	Finish(p, "ok")
	c.expire()
	if c.pipeOwnerProbes.Load() != PipeOwnerProbeLimit {
		t.Fatal("exhausted counter kept growing")
	}
}

func TestPipeOwnerCyclesStayWithMatchingInputAndMeasuredZero(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	Backend(p, "installed")
	x, complete := Input(ctx, "receiver")
	RecordPipeOwnerCycles(x, 99) // no matching wall span yet
	if x.Value(sampleKey{}).(*sample).vector.PipeOwnerCyclesMeasured {
		t.Fatal("cycles without matching wall span")
	}
	if !WantPipeOwnerCycles(x) {
		t.Fatal("fresh input probe unavailable")
	}
	RecordAuthority(x, AuthorityPipeOwner, 11*time.Millisecond)
	RecordPipeOwnerCycles(x, 0)
	RecordPipeOwnerCycles(x, 99)
	complete(nil)
	x, complete = Input(ctx, "receiver")
	RecordAuthority(x, AuthorityPipeOwner, time.Millisecond)
	RecordPipeOwnerCycles(x, 1234)
	complete(nil)
	Finish(p, "ok")
	s := (*out)[0]
	if !s.Valid() || !s.First.PipeOwnerCyclesMeasured || s.First.PipeOwnerCycles != 0 || !s.Worst.PipeOwnerCyclesMeasured || s.Worst.PipeOwnerCycles != 1234 {
		t.Fatal("measured zero, immutable first or matching worst lost")
	}
	c.expire()
	if WantPipeOwnerCycles(x) {
		t.Fatal("stopped collector kept probing")
	}
	for _, bad := range []struct {
		cycles   uint64
		measured bool
		presence uint8
	}{{1, false, 2}, {1, true, 0}, {MaxPipeOwnerCycles + 1, true, 2}} {
		if ValidPipeOwnerCycles(bad.cycles, bad.measured, bad.presence) {
			t.Fatal("invalid cycle presence/range accepted")
		}
	}
}
