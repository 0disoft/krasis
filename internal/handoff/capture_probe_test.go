package handoff

import (
	"context"
	"testing"
	"time"
)

func TestCaptureProbeRetainsOwnerStagesAndSealsLateCompletion(t *testing.T) {
	ctx, probe := WithCaptureProbe(context.Background())
	RecordCapture(ctx, CaptureForeground, 321*time.Millisecond)
	RecordCapture(ctx, CaptureShow, 0)
	RecordCapture(ctx, CaptureForeground, time.Second)  // First observation wins.
	RecordCapture(ctx, CaptureIPCResponse, time.Second) // Agent spans cannot enter owner detail.
	span := CaptureSpan(ctx, CaptureFallback)
	native := probe.Finish()
	span() // A timed-out owner may finish after the reply is frozen.
	if !native.Valid() || native[CaptureForeground] != 4 || native[CaptureShow] != 1 || native[CaptureFallback] != 0 || probe.Finish() != native {
		t.Fatal("owner timing lost presence, mixed spans or changed after seal", native)
	}
}

func TestCaptureNativeDetailsRequireMatchingReplyAndRemainFrozen(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "source", "first", "epoch", time.Now())
	var native CaptureNative
	native[CaptureForeground] = 4
	RecordCaptureWorkerDetails(p, [3]byte{0, 3, 4}, native) // No response observation yet.
	RecordCapture(p, CaptureIPCResponse, 324*time.Millisecond)
	bad := native
	bad[CaptureShow] = 6
	RecordCaptureWorkerDetails(p, [3]byte{0, 3, 4}, bad)
	RecordCaptureWorkerDetails(p, [3]byte{0, 3, 4}, native)
	Finish(p, "ok")
	before := *(*out)[0].Capture
	RecordCaptureWorkerDetails(p, [3]byte{0, 5, 5}, CaptureNative{5})
	if !before.Valid() || before.Native != native || before.Worker != ([3]byte{0, 3, 4}) || *(*out)[0].Capture != before {
		t.Fatal("foreign, invalid or late detail contaminated the attempt", before)
	}
}
