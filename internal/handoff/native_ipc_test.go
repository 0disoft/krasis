package handoff

import (
	"testing"
	"time"
)

func TestNativeIPCProfileBoundariesAndSampleOwnership(t *testing.T) {
	limit := uint32(Window.Microseconds())
	if !(NativeIPCProfile{Micros: [6]uint32{limit}}).Valid() || (NativeIPCProfile{Micros: [6]uint32{limit, 1}}).Valid() {
		t.Fatal("IPC total observation bound changed")
	}
	_, ctx, out := collected(t)
	a := Begin(ctx, "receiver", "s", "e", time.Now())
	Backend(a, "installed")
	x, complete := Input(ctx, "receiver")
	if !WantWorkerApply(x) {
		t.Fatal("missing sample")
	}
	detail := WorkerApplyProfile{Micros: [2]uint32{1000}, Measured: 1, NativeIPC: &NativeIPCProfile{Micros: [6]uint32{1, 2, 3, 4, 5, 6}}}
	RecordWorkerApply(x, &detail)
	detail.NativeIPC.Micros[0] = limit + 1
	complete(nil)
	Finish(a, "ok")
	if len(*out) != 1 || !(*out)[0].Valid() || (*out)[0].First.WorkerApply.NativeIPC.Micros[0] != 1 {
		t.Fatal("worker response mutated saved diagnostic")
	}
}

func TestNativeIPCEnvelopeMayExceedChildApply(t *testing.T) {
	p := WorkerApplyProfile{Micros: [2]uint32{184560}, Measured: 1,
		NativeIPC: &NativeIPCProfile{Micros: [6]uint32{1000000, 100000, 10000, 5000, 1000, 742}}}
	if !p.Valid() || !p.ValidForWorker([3]uint16{0, 0, 185}, 4) {
		t.Fatal("parent-side stall incorrectly rejected against child apply")
	}
}

func TestNativeIPCParentPresenceAndTotalBudget(t *testing.T) {
	limit := uint32(Window.Microseconds())
	if !(NativeIPCProfile{ParentMeasured: true, ParentMicros: [3]uint32{limit}}).Valid() ||
		(NativeIPCProfile{ParentMicros: [3]uint32{1}}).Valid() ||
		(NativeIPCProfile{Micros: [6]uint32{1}, ParentMeasured: true, ParentMicros: [3]uint32{limit}}).Valid() {
		t.Fatal("parent presence or containing window budget lost")
	}
}
