package handoff

import (
	"context"
	"sync"
	"time"
)

// The existing native stage order ends before agent-side IPC measurements.
// Zero is unobserved; 1..5 are <1ms, <10ms, <100ms, <1s and >=1s.
type CaptureNative [CaptureIPCLock]byte

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

func (p *CaptureProbe) Finish() CaptureNative {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sealed = true
	return p.native
}
