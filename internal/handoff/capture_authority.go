package handoff

import (
	"context"
	"time"
)

// Nested authority elapsed: native query execution versus admission, query
// startup, result publication and caller resumption. Native still includes
// scheduling during the query; this is not native CPU time.
type CaptureAuthorityProfile struct {
	Measured     bool   `json:"measured"`
	WaitMicros   uint32 `json:"waitMicros"`
	NativeMicros uint32 `json:"nativeMicros"`
}

func (p CaptureAuthorityProfile) Valid() bool {
	if !p.Measured {
		return p == (CaptureAuthorityProfile{})
	}
	return uint64(p.WaitMicros)+uint64(p.NativeMicros) <= uint64(Window.Microseconds())
}

func RecordCaptureAuthority(ctx context.Context, detail CaptureAuthorityProfile) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil || !detail.Measured || !detail.Valid() {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if a.finished || s.collector.stopped.Load() || time.Since(s.collector.started) >= Window || a.summary.Role != "source" {
		return
	}
	p := a.summary.Capture
	if p != nil && p.Measured&(1<<CaptureIPCAuthority) != 0 && !p.Authority.Measured {
		p.Authority = detail
	}
}
