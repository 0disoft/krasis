package handoff

import (
	"testing"
	"time"
)

func TestCaptureDetailFreezesAndLateOwnerCannotPolluteNextAttempt(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "source", "first", "epoch", time.Now())
	RecordCapture(p, CaptureShow, 0)
	RecordCapture(p, CaptureForeground, 140*time.Millisecond)
	RecordCapture(p, CaptureForeground, time.Second)
	RecordCapture(p, CaptureStageCount, time.Second)
	Finish(p, "incomplete")
	before := *(*out)[0].Capture
	next := Begin(ctx, "source", "next", "epoch", time.Now())
	done := make(chan struct{})
	go func() {
		RecordCapture(p, CaptureResultWait, time.Second)
		close(done)
	}()
	<-done
	RecordCapture(next, CaptureShow, time.Millisecond)
	Finish(next, "incomplete")
	if *(*out)[0].Capture != before || before.Micros[CaptureForeground] != 140000 || before.Measured != 1<<CaptureShow|1<<CaptureForeground || (*out)[1].Capture.Measured != 1<<CaptureShow {
		t.Fatal("late owner changed retired or later capture evidence", *out)
	}
	for _, mutate := range []func(*CaptureProfile){
		func(p *CaptureProfile) { p.Schema = 2 },
		func(p *CaptureProfile) { p.Measured |= 1 << CaptureStageCount },
		func(p *CaptureProfile) { p.Micros[CapturePost] = 1 },
		func(p *CaptureProfile) { p.Micros[CaptureShow] = uint32(Window.Microseconds() + 1) },
		func(p *CaptureProfile) { p.Worker = [3]byte{0, 3, 4} }, // Missing matching response span.
		func(p *CaptureProfile) { p.Worker[0] = 1 },
		func(p *CaptureProfile) { p.Worker[2] = 6 },
	} {
		bad := before
		mutate(&bad)
		if bad.Valid() {
			t.Fatal("invalid capture detail accepted")
		}
	}
}

func TestCaptureWorkerDetailKeepsOneMatchingAttempt(t *testing.T) {
	c, ctx, out := collected(t)
	p := Begin(ctx, "source", "first", "epoch", time.Now())
	RecordCapture(p, CaptureIPCResponse, 117*time.Millisecond)
	RecordCaptureWorker(p, [3]byte{1, 3, 4}) // Reader phase is not measured.
	RecordCaptureWorker(p, [3]byte{0, 3, 4})
	RecordCaptureWorker(p, [3]byte{0, 1, 1}) // Never replace a recorded ACK.
	Finish(p, "ok")
	before := *(*out)[0].Capture
	next := Begin(ctx, "source", "second", "epoch", time.Now())
	RecordCapture(next, CaptureIPCResponse, time.Millisecond)
	RecordCaptureWorker(p, [3]byte{0, 5, 5}) // Late completion is fenced.
	c.started = time.Now().Add(-Window)
	RecordCaptureWorker(next, [3]byte{0, 2, 2})
	Finish(next, "incomplete")
	if !before.Valid() || before.Worker != ([3]byte{0, 3, 4}) || *(*out)[0].Capture != before || (*out)[1].Capture.Worker != ([3]byte{}) {
		t.Fatal("ACK detail overwritten, crossed attempts or escaped expiry", *out)
	}
}

func TestCaptureDetailDropsReceiverAndExpiredOrMissingContexts(t *testing.T) {
	RecordCapture(nil, CaptureShow, time.Second)
	CaptureSpan(nil, CaptureShow)()
	c, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	RecordCapture(p, CaptureShow, time.Second)
	Finish(p, "incomplete")
	if (*out)[0].Capture != nil {
		t.Fatal("receiver accepted source capture detail")
	}
	p = Begin(ctx, "source", "session", "epoch", time.Now())
	RecordCapture(p, CaptureShow, -time.Second)
	RecordCapture(p, CaptureUpdate, Window+time.Second)
	c.started = time.Now().Add(-Window)
	RecordCapture(p, CaptureForeground, time.Second)
	Finish(p, "incomplete")
	got := (*out)[1].Capture
	if got.Measured != 1<<CaptureShow|1<<CaptureUpdate || got.Micros[CaptureShow] != 0 || got.Micros[CaptureUpdate] != uint32(Window.Microseconds()) {
		t.Fatal("capture presence, clamp or expiry changed", got)
	}
}
