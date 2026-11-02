package handoff

import (
	"context"
	"time"
)

// At most 1000 tracked commands/second over a 120s diagnostic window.
// Every admitted command is measured: selecting it before execution cannot
// predict which unsampled command will become the entry's worst input.
const WorkerApplyProbeLimit = 120000

// WorkerApplyProfile contains two disjoint worker-local parts of WorkerApply:
// the existing apply function and the subsequent cursor snapshot. It records
// durations only, never operation kinds, input values or cursor coordinates.
type WorkerApplyProfile struct {
	NativeIPC *NativeIPCProfile `json:"nativeIPC,omitempty"`
	Micros    [2]uint32         `json:"micros"`
	Measured  uint8             `json:"measured"`
	Saturated uint8             `json:"saturated,omitempty"`
	// Submission is nested inside apply, not an additional disjoint part.
	// Calls is 0 (absent), 1 (measured), or 2 (ambiguous; drop diagnostics).
	SubmissionMicros    uint32 `json:"submissionMicros,omitempty"`
	SubmissionCalls     uint8  `json:"submissionCalls,omitempty"`
	SubmissionSaturated bool   `json:"submissionSaturated,omitempty"`
}

func (p WorkerApplyProfile) Valid() bool {
	// IPC includes parent executor/transport wait; it is not nested in apply.
	if p.NativeIPC != nil && (!p.NativeIPC.Valid() || p.Measured&1 == 0) {
		return false
	}
	if p.Measured & ^uint8(3) != 0 || p.Saturated & ^p.Measured != 0 {
		return false
	}
	for i, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || p.Measured&(1<<i) == 0 && n != 0 || p.Saturated&(1<<i) != 0 && n != uint32(Window.Microseconds()) {
			return false
		}
	}
	if p.SubmissionCalls > 1 || p.SubmissionCalls == 0 && (p.SubmissionMicros != 0 || p.SubmissionSaturated) {
		return false
	}
	if p.SubmissionCalls == 1 && (p.Measured&1 == 0 || p.SubmissionMicros > p.Micros[0] || p.SubmissionSaturated && (p.SubmissionMicros != uint32(Window.Microseconds()) || p.Saturated&1 == 0)) {
		return false
	}
	return true
}

// When the containing span is present and unsaturated, disjoint parts must fit
// below its next millisecond boundary. Do not derive a scheduler residual.
func (p WorkerApplyProfile) ValidForWorker(millis [3]uint16, measured uint8) bool {
	if !p.Valid() {
		return false
	}
	if measured&4 != 0 && millis[2] < MaxWorkerMillis {
		return uint64(p.Micros[0])+uint64(p.Micros[1]) < uint64(millis[2]+1)*1000
	}
	return true
}

func (p *WorkerApplyProfile) Record(slot int, elapsed time.Duration) {
	if p == nil || slot < 0 || slot >= len(p.Micros) || p.Measured&(1<<slot) != 0 {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
		p.Saturated |= 1 << slot
	}
	p.Micros[slot] = uint32(elapsed.Microseconds())
	p.Measured |= 1 << slot
}

// Measure tracked commands throughout the bounded window, retaining only the
// existing first/worst summaries. This adds numeric suffixes to existing local
// exchanges, never a request, native submission or additional round trip.
// The cap spans all reentries/reconnects within a window. Rolling collection
// renews it only after freezing every old attempt; stale contexts stay inert.
func WantWorkerApply(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(sampleKey{}).(*sample)
	s, a := getSession(ctx), getAttempt(ctx)
	if v == nil || s == nil || a == nil || s.collector == nil || s.collector.stopped.Load() {
		return false
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if v.workerApplyRequested || a.finished || a.summary.Role != "receiver" || a.summary.Backend != "installed" || time.Since(s.collector.started) >= Window || s.collector.workerApplyProbes >= WorkerApplyProbeLimit {
		return false
	}
	s.collector.workerApplyProbes++
	v.workerApplyRequested = true
	return true
}

// The client invokes this only after matching the response sequence and op.
// Later recovery responses cannot replace the original sample's diagnostic.
func RecordWorkerApply(ctx context.Context, p *WorkerApplyProfile) {
	if ctx == nil {
		return
	}
	v, _ := ctx.Value(sampleKey{}).(*sample)
	if v == nil || !v.workerApplyRequested || v.workerApplyRecorded {
		return
	}
	v.workerApplyRecorded = true
	if p != nil && p.ValidForWorker(v.vector.WorkerMillis, v.vector.WorkerMeasured) && p.Measured != 0 {
		copy := *p
		if p.NativeIPC != nil {
			ipc := *p.NativeIPC
			copy.NativeIPC = &ipc
		}
		v.vector.WorkerApply = &copy
	}
}
