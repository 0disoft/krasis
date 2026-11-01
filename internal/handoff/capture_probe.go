package handoff

import (
	"context"
	"sync"
	"time"
)

// The existing native stage order ends before agent-side IPC measurements.
// Zero is unobserved; 1..5 are <1ms, <10ms, <100ms, <1s and >=1s.
type CaptureNative [CaptureIPCLock]byte

const CaptureExactStageCount = 3

type CaptureExactStage uint8

const (
	CaptureExactQueueToHandle CaptureExactStage = iota
	CaptureExactForegroundRequest
	CaptureExactForegroundVerify
)

// CaptureNativeExact keeps only the three owner spans needed to separate the
// activation command wait from the foreground API call and its verification.
// Measured is authoritative; a measured zero-duration span remains zero here.
type CaptureNativeExact struct {
	QueueToHandleMicros     uint32 `json:"queueToHandleMicros,omitempty"`
	ForegroundRequestMicros uint32 `json:"foregroundRequestMicros,omitempty"`
	ForegroundVerifyMicros  uint32 `json:"foregroundVerifyMicros,omitempty"`
	Measured                uint8  `json:"measured,omitempty"`
}

func (p CaptureNativeExact) Valid() bool {
	if p.Measured>>CaptureExactStageCount != 0 {
		return false
	}
	limit := uint32(Window.Microseconds())
	for stage := CaptureExactStage(0); stage < CaptureExactStageCount; stage++ {
		value := p.value(stage)
		if value > limit || p.Measured&(1<<stage) == 0 && value != 0 {
			return false
		}
	}
	return true
}

func (p CaptureNativeExact) value(stage CaptureExactStage) uint32 {
	switch stage {
	case CaptureExactQueueToHandle:
		return p.QueueToHandleMicros
	case CaptureExactForegroundRequest:
		return p.ForegroundRequestMicros
	case CaptureExactForegroundVerify:
		return p.ForegroundVerifyMicros
	default:
		return 0
	}
}

func (p *CaptureNativeExact) setValue(stage CaptureExactStage, value uint32) {
	switch stage {
	case CaptureExactQueueToHandle:
		p.QueueToHandleMicros = value
	case CaptureExactForegroundRequest:
		p.ForegroundRequestMicros = value
	case CaptureExactForegroundVerify:
		p.ForegroundVerifyMicros = value
	}
}

func (p *CaptureNativeExact) record(stage CaptureExactStage, elapsed time.Duration) {
	if stage >= CaptureExactStageCount || p.Measured&(1<<stage) != 0 {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > Window {
		elapsed = Window
	}
	p.setValue(stage, uint32(elapsed.Microseconds()))
	p.Measured |= 1 << stage
}

// Wire returns the three fixed uint32 activation-ACK values. One is stored so
// zero remains unobserved even when the measured duration rounds to zero.
func (p CaptureNativeExact) Wire() [CaptureExactStageCount]uint32 {
	var values [CaptureExactStageCount]uint32
	for stage := CaptureExactStage(0); stage < CaptureExactStageCount; stage++ {
		if p.Measured&(1<<stage) != 0 {
			values[stage] = p.value(stage) + 1
		}
	}
	return values
}

func CaptureNativeExactFromWire(values [CaptureExactStageCount]uint32) (CaptureNativeExact, bool) {
	var p CaptureNativeExact
	for stage, encoded := range values {
		if encoded == 0 {
			continue
		}
		p.setValue(CaptureExactStage(stage), encoded-1)
		p.Measured |= 1 << stage
	}
	return p, p.Valid()
}

func (p CaptureNative) Valid() bool {
	for _, bucket := range p {
		if bucket > 5 {
			return false
		}
	}
	return true
}

type captureProbeKey struct{}

// CaptureProbe collects only fixed timing buckets inside the isolated owner.
// It holds no session IDs, window handles, input data or authority. Finish seals
// the original command so a late owner cannot mutate its ACK or the next call.
type CaptureProbe struct {
	mu     sync.Mutex
	sealed bool
	native CaptureNative
	exact  CaptureNativeExact
}

func WithCaptureProbe(ctx context.Context) (context.Context, *CaptureProbe) {
	probe := &CaptureProbe{}
	return context.WithValue(ctx, captureProbeKey{}, probe), probe
}

func captureProbeFrom(ctx context.Context) *CaptureProbe {
	if ctx == nil {
		return nil
	}
	probe, _ := ctx.Value(captureProbeKey{}).(*CaptureProbe)
	return probe
}

func (p *CaptureProbe) record(stage CaptureStage, elapsed time.Duration) {
	if stage >= CaptureIPCLock {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sealed || p.native[stage] != 0 {
		return
	}
	bucket := byte(1)
	for _, limit := range [...]time.Duration{time.Millisecond, 10 * time.Millisecond, 100 * time.Millisecond, time.Second} {
		if elapsed < limit {
			break
		}
		bucket++
	}
	p.native[stage] = bucket
}

func (p *CaptureProbe) recordExact(stage CaptureExactStage, elapsed time.Duration) {
	if stage >= CaptureExactStageCount {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sealed {
		return
	}
	p.exact.record(stage, elapsed)
}

func (p *CaptureProbe) Finish() CaptureNative {
	native, _ := p.FinishDetailed()
	return native
}

func (p *CaptureProbe) FinishDetailed() (CaptureNative, CaptureNativeExact) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sealed = true
	return p.native, p.exact
}

// RecordCaptureExact is owner-only diagnostics. It never creates authority and
// is ignored outside the matching activation probe context.
func RecordCaptureExact(ctx context.Context, stage CaptureExactStage, elapsed time.Duration) {
	if probe := captureProbeFrom(ctx); probe != nil {
		probe.recordExact(stage, elapsed)
		return
	}
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || stage >= CaptureExactStageCount {
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
	a.summary.Capture.Exact.record(stage, elapsed)
}
