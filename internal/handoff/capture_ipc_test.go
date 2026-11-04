package handoff

import (
	"testing"
	"time"
)

func TestCaptureIPCPartitionsClockChainAndRejectsInvalidClocks(t *testing.T) {
	clocks := [9]uint64{100, 110, 130, 160, 200, 250, 310, 380, 460}
	p := CaptureIPCFromClocks(clocks)
	if !p.Valid() || !p.Measured || p.RequestDeliveryMicros != 10 || p.ActivationMicros != 40 || p.ReplyDeliveryMicros != 70 || p.CallerResumeMicros != 80 {
		t.Fatal(p)
	}
	for i := range clocks {
		bad := clocks
		bad[i] = 0
		if CaptureIPCFromClocks(bad) != (CaptureIPC{}) {
			t.Fatal("missing clock accepted", i)
		}
		if i > 0 {
			bad = clocks
			bad[i] = bad[i-1] - 1
			if CaptureIPCFromClocks(bad) != (CaptureIPC{}) {
				t.Fatal("reversed clock accepted", i)
			}
		}
	}
	for _, delta := range []uint64{uint64(Window.Microseconds()) - 1, uint64(Window.Microseconds()), uint64(Window.Microseconds()) + 1, ^uint64(0) - 100} {
		edge := [9]uint64{100, 100, 100, 100, 100, 100, 100, 100, 100 + delta}
		if got := CaptureIPCFromClocks(edge); got.Measured != (delta <= uint64(Window.Microseconds())) {
			t.Fatal("window boundary", delta, got)
		}
	}
	if p := CaptureIPCFromClocks([9]uint64{1, 1, 1, 1, 1, 1, 1, 1, 1}); !p.Measured || !p.Valid() {
		t.Fatal("observed zero lost", p)
	}
}

func TestCaptureIPCRequiresMatchingResponseAndFreezesAtFinish(t *testing.T) {
	_, ctx, out := collected(t)
	a := Begin(ctx, "source", "session", "epoch", time.Now())
	ipc := CaptureIPCFromClocks([9]uint64{1, 2, 3, 4, 5, 6, 7, 8, 9})
	RecordCapture(a, CaptureIPCWrite, time.Microsecond)
	RecordCaptureIPC(a, ipc) // No matching decoded reply yet.
	RecordCapture(a, CaptureIPCResponse, 8*time.Microsecond)
	RecordCaptureIPC(a, ipc)
	other := ipc
	other.CallerResumeMicros++
	RecordCaptureIPC(a, other)
	Finish(a, "ok")
	RecordCaptureIPC(a, other)
	if len(*out) != 1 || (*out)[0].Capture.IPC != ipc {
		t.Fatal("missing/duplicate/late chain altered profile", *out)
	}
	invalid := *(*out)[0].Capture
	invalid.Measured &^= 1 << CaptureIPCResponse
	if invalid.Valid() {
		t.Fatal("IPC accepted without matching response")
	}
}
