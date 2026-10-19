package handoff

import (
	"testing"
	"time"
)

func TestPipeOpenPartitionsOnlyOneInstalledReceiverAttempt(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	Backend(p, "installed")
	profile := PipeOpenProfile{Schema: 1, Attempts: 3, Busy: 1, Missing: 1, OpenMicros: 500, WaitMicros: 7000}
	RecordPipeOpen(p, profile)
	Record(p, PipeOpen, 8*time.Millisecond)
	RecordPipeOpen(p, PipeOpenProfile{Schema: 1, Attempts: 1})
	Finish(p, "ok")
	if len(*out) != 1 || (*out)[0].PipeOpen == nil || *(*out)[0].PipeOpen != profile {
		t.Fatal("pipe evidence lost or overwritten")
	}
	RecordPipeOpen(p, PipeOpenProfile{Schema: 1, Attempts: 1})
	if *(*out)[0].PipeOpen != profile {
		t.Fatal("finished evidence changed")
	}
}

func TestPipeOpenRejectsImpossibleCountsTimesAndRoles(t *testing.T) {
	base := Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok", PipeOpen: &PipeOpenProfile{Schema: 1, Attempts: 2, Missing: 1, OpenMicros: 500, WaitMicros: 7000}}
	base.Preparation.Measured = 1 << PipeOpen
	base.Preparation.Micros[PipeOpen] = 8000
	if !base.Valid() {
		t.Fatal("valid profile rejected")
	}
	for _, bad := range []PipeOpenProfile{
		{Schema: 1}, {Schema: 2, Attempts: 1}, {Schema: 1, Attempts: 65536},
		{Schema: 1, Attempts: 2, Missing: 2, Busy: 1},
		{Schema: 1, Attempts: 2, WaitMicros: 9000},
	} {
		v := base
		v.PipeOpen = &bad
		if v.Valid() {
			t.Fatal("impossible evidence accepted", bad)
		}
	}
	base.Role, base.Backend = "source", "none"
	if base.Valid() {
		t.Fatal("source can report receiver pipe evidence")
	}
}
