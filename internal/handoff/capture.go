package handoff

import (
	"context"
	"time"
)

// Local source-capture spans. Nested waits are not additive; these never
// participate in focus validation, command ordering or input authority.
type CaptureStage uint8

const (
	CaptureSerializeWait CaptureStage = iota
	CaptureAdmission
	CapturePost
	CaptureQueueToHandle
	CapturePosition
	CaptureTouchpadRegister
	CaptureShow
	CaptureUpdate
	CaptureForeground
	CaptureFallback
	CaptureActiveCommit
	CaptureHandlerTotal
	CaptureResultWait
	CaptureRollback
	CaptureTotal
	CaptureBoundaryBefore
	CaptureBoundaryAfter
	CaptureIPCLock
	CaptureIPCAuthority
	CaptureIPCWrite
	CaptureIPCResponse
	CaptureStageCount
)

type CaptureProfile struct {
	Schema   uint8                     `json:"schema"`
	Micros   [CaptureStageCount]uint32 `json:"micros"`
	Measured uint32                    `json:"measured"`
	Worker   [3]byte                   `json:"worker"`          // Matching activation ACK buckets; 0 is unobserved.
	Native   CaptureNative             `json:"native,omitzero"` // Matching owner-thread stages, separate from agent spans.
}

func (p CaptureProfile) Valid() bool {
	if p.Schema != 1 || p.Measured>>CaptureStageCount != 0 {
		return false
	}
	if p.Worker[0] != 0 || !p.Native.Valid() || (p.Worker != ([3]byte{}) || p.Native != (CaptureNative{})) && p.Measured&(1<<CaptureIPCResponse) == 0 {
		return false
	}
	for _, bucket := range p.Worker {
		if bucket > 5 {
			return false
		}
	}
	for i, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || (p.Measured&(1<<i) == 0 && n != 0) {
			return false
		}
	}
	return true
}

// Attach only a fully decoded, matching activation reply, including a known
// rejection. This preserves the original attempt rather than a 15s maximum
// from unrelated admit/activate/heartbeat operations. No wire probe is added.
func RecordCaptureWorker(ctx context.Context, worker [3]byte) {
	RecordCaptureWorkerDetails(ctx, worker, CaptureNative{})
}

func RecordCaptureWorkerDetails(ctx context.Context, worker [3]byte, native CaptureNative) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || worker == ([3]byte{}) && native == (CaptureNative{}) {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if a.finished || s.collector.stopped.Load() || time.Since(s.collector.started) >= Window || a.summary.Role != "source" || a.summary.Capture == nil {
		return
	}
	p := a.summary.Capture
	if p.Worker != ([3]byte{}) || p.Native != (CaptureNative{}) {
		return
	}
	next := *p
	next.Worker = worker
	next.Native = native
	if next.Valid() {
		*p = next
	}
}

// Commands retain the original attempt context across the local owner thread.
// Retire freezes that attempt before emitting, so late command completion can
// neither mutate its receipt nor attach measurements to a later attempt.
func RecordCapture(ctx context.Context, stage CaptureStage, elapsed time.Duration) {
	if probe := captureProbeFrom(ctx); probe != nil {
		probe.record(stage, elapsed)
		return
	}
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || stage >= CaptureStageCount {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if a.finished || s.collector.stopped.Load() || time.Since(s.collector.started) >= Window || a.summary.Role != "source" {
		return
	}
	if a.summary.Capture == nil {
		a.summary.Capture = &CaptureProfile{Schema: 1}
	}
	p := a.summary.Capture
	if p.Measured&(1<<stage) != 0 {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
	}
	p.Micros[stage] = uint32(elapsed.Microseconds())
	p.Measured |= 1 << stage
}

func CaptureSpan(ctx context.Context, stage CaptureStage) func() {
	if captureProbeFrom(ctx) != nil && stage < CaptureIPCLock {
		started := time.Now()
		return func() { RecordCapture(ctx, stage, time.Since(started)) }
	}
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || stage >= CaptureStageCount || s.collector.stopped.Load() {
		return func() {}
	}
	started := time.Now()
	return func() { RecordCapture(ctx, stage, time.Since(started)) }
}
