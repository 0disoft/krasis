package handoff

import (
	"testing"
	"time"
)

func TestCaptureAuthorityBoundsAndAttemptOwnership(t *testing.T) {
	limit := uint32(Window.Microseconds())
	for _, p := range []CaptureAuthorityProfile{
		{}, {Measured: true}, {Measured: true, WaitMicros: limit}, {Measured: true, WaitMicros: limit - 1, NativeMicros: 1},
	} {
		if !p.Valid() {
			t.Fatal("valid boundary lost", p)
		}
	}
	for _, p := range []CaptureAuthorityProfile{
		{WaitMicros: 1}, {Measured: true, WaitMicros: limit, NativeMicros: 1}, {Measured: true, WaitMicros: ^uint32(0), NativeMicros: 1},
	} {
		if p.Valid() {
			t.Fatal("invalid or overflowing sum accepted", p)
		}
	}
	_, ctx, out := collected(t)
	a := Begin(ctx, "source", "session", "epoch", time.Now())
	first := CaptureAuthorityProfile{Measured: true, WaitMicros: 10, NativeMicros: 20}
	RecordCaptureAuthority(a, first) // No matching authority stage yet.
	RecordCapture(a, CaptureIPCAuthority, 30*time.Microsecond)
	RecordCaptureAuthority(a, first)
	RecordCaptureAuthority(a, CaptureAuthorityProfile{Measured: true, NativeMicros: 99})
	Finish(a, "ok")
	RecordCaptureAuthority(a, CaptureAuthorityProfile{Measured: true, NativeMicros: 99})
	if len(*out) != 1 || (*out)[0].Capture.Authority != first || !(*out)[0].Capture.Valid() {
		t.Fatal("missing/late authority changed attempt", *out)
	}
	bad := *(*out)[0].Capture
	bad.Measured = 0
	if bad.Valid() {
		t.Fatal("unmatched authority detail accepted")
	}
	ipc := CaptureIPC{ReadAuthority: first}
	if ipc.Valid() {
		t.Fatal("detail without a matched IPC chain accepted")
	}
}
