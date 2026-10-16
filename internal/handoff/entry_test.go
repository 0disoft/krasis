package handoff

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestEntryKeepsMatchingACKAndFailureWithoutConsumingFirstInput(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "s", "e", time.Now())
	Backend(p, "installed")
	x, done := Entry(p)
	Record(x, ClientResponse, 1935*time.Millisecond)
	WorkerDetail(x, [3]byte{1, 2, 4}, [3]uint16{0, 1, 900}, 7)
	if !WantWorkerApply(x) {
		t.Fatal("entry omitted worker detail")
	}
	detail := WorkerApplyProfile{Micros: [2]uint32{900000, 0}, Measured: 1}
	RecordWorkerApply(x, &detail)
	done(errors.New("test failure"))
	done(nil)
	_, duplicate := Entry(p)
	duplicate(nil)
	i, finish := Input(ctx, "receiver")
	Record(i, ClientResponse, time.Millisecond)
	finish(nil)
	Finish(p, "error")
	s := (*out)[0]
	if !s.Valid() || s.Entry == nil || s.Entry.Outcome != "error" || s.Entry.Micros[ClientResponse] != 1935000 || s.Entry.WorkerMillis[2] != 900 || s.Entry.WorkerApply == nil || s.Entry.WorkerApply.Micros[0] != 900000 || c.workerApplyProbes != 1 {
		t.Fatal("entry lost its matching detail", s)
	}
	if s.Frames != 0 || s.First.Outcome != "ok" || s.First.Micros[ClientResponse] != 1000 || s.First.WorkerApply != nil {
		t.Fatal("preparation contaminated input samples", s)
	}
	bad := s
	bad.Role, bad.Backend = "source", "none"
	if bad.Valid() {
		t.Fatal("source exported entry details")
	}
	bad = s
	v := *s.Entry
	v.WorkerMeasured = 8
	bad.Entry = &v
	if bad.Valid() {
		t.Fatal("invalid entry worker metadata accepted")
	}
}

func TestEntryRejectsUntrackedAndLateRetiredCompletion(t *testing.T) {
	if x, _ := Entry(context.Background()); WantWorkerApply(x) {
		t.Fatal("untracked entry requested a probe")
	}
	c, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "s", "e", time.Now())
	Backend(p, "installed")
	x, done := Entry(p)
	Record(x, ClientResponse, time.Second)
	c.expire()
	done(nil)
	if len(*out) != 1 || (*out)[0].Entry != nil {
		t.Fatal("late entry altered frozen diagnostics")
	}
	if WantWorkerApply(x) {
		t.Fatal("expired entry requested a probe")
	}
}
