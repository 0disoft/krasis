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
	RecordCaptureExact(ctx, CaptureExactQueueToHandle, 7*time.Millisecond)
	RecordCaptureExact(ctx, CaptureExactForegroundRequest, 3*time.Millisecond)
	RecordCaptureExact(ctx, CaptureExactForegroundVerify, 4*time.Millisecond)
	RecordCaptureExact(ctx, CaptureExactQueueToHandle, time.Second) // First observation wins.
	span := CaptureSpan(ctx, CaptureFallback)
	native, exact := probe.FinishDetailed()
	span() // A timed-out owner may finish after the reply is frozen.
	if !native.Valid() || native[CaptureForeground] != 4 || native[CaptureShow] != 1 || native[CaptureFallback] != 0 || probe.Finish() != native {
		t.Fatal("owner timing lost presence, mixed spans or changed after seal", native)
	}
	if !exact.Valid() || exact.Measured != 1<<CaptureExactQueueToHandle|1<<CaptureExactForegroundRequest|1<<CaptureExactForegroundVerify || exact.QueueToHandleMicros != 7000 || exact.ForegroundRequestMicros != 3000 || exact.ForegroundVerifyMicros != 4000 {
		t.Fatal("exact owner timing lost first observations or measured state", exact)
	}
	if _, after := probe.FinishDetailed(); after != exact {
		t.Fatal("late exact timing changed sealed probe", after)
	}
}

func TestCaptureNativeExactWireUsesZeroSentinelAndValidatedBounds(t *testing.T) {
	exact := CaptureNativeExact{Measured: 1<<CaptureExactQueueToHandle | 1<<CaptureExactForegroundRequest | 1<<CaptureExactForegroundVerify}
	exact.QueueToHandleMicros = 0
	exact.ForegroundRequestMicros = 12
	exact.ForegroundVerifyMicros = uint32(Window.Microseconds())
	wire := exact.Wire()
	if wire[CaptureExactQueueToHandle] != 1 || wire[CaptureExactForegroundRequest] != 13 || wire[CaptureExactForegroundVerify] != uint32(Window.Microseconds())+1 {
		t.Fatal("exact wire lost micros+1 encoding", wire)
	}
	got, ok := CaptureNativeExactFromWire(wire)
	if !ok || got != exact {
		t.Fatal("exact wire round trip changed measured detail", got, ok)
	}
	legacy, ok := CaptureNativeExactFromWire([CaptureExactStageCount]uint32{})
	if !ok || legacy != (CaptureNativeExact{}) {
		t.Fatal("legacy zero detail became observed", legacy, ok)
	}
	for _, invalid := range []CaptureNativeExact{
		{Measured: 1 << CaptureExactStageCount},
		{QueueToHandleMicros: 1},
		{ForegroundRequestMicros: uint32(Window.Microseconds()) + 1, Measured: 1 << CaptureExactForegroundRequest},
	} {
		if invalid.Valid() {
			t.Fatal("invalid exact detail accepted", invalid)
		}
	}
}

func TestCaptureNativeDetailsRequireMatchingReplyAndRemainFrozen(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "source", "first", "epoch", time.Now())
	var native CaptureNative
	native[CaptureQueueToHandle] = 3
	native[CaptureForeground] = 4
	exact := CaptureNativeExact{QueueToHandleMicros: 7, ForegroundRequestMicros: 11, ForegroundVerifyMicros: 13, Measured: 1<<CaptureExactQueueToHandle | 1<<CaptureExactForegroundRequest | 1<<CaptureExactForegroundVerify}
	RecordCaptureWorkerDetails(p, [3]byte{0, 3, 4}, native) // No response observation yet.
	RecordCapture(p, CaptureIPCResponse, 324*time.Millisecond)
	bad := native
	bad[CaptureShow] = 6
	RecordCaptureWorkerDetails(p, [3]byte{0, 3, 4}, bad)
	RecordCaptureWorkerExactDetails(p, [3]byte{0, 3, 4}, native, exact)
	Finish(p, "ok")
	before := *(*out)[0].Capture
	RecordCaptureWorkerExactDetails(p, [3]byte{0, 5, 5}, CaptureNative{5}, exact)
	if !before.Valid() || before.Native != native || before.Exact != exact || before.Worker != ([3]byte{0, 3, 4}) || *(*out)[0].Capture != before {
		t.Fatal("foreign, invalid or late detail contaminated the attempt", before)
	}
}
