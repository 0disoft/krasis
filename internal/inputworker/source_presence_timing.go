package inputworker

import (
	"context"
	"github.com/0disoft/krasis/internal/handoff"
	"time"
)

type sourcePresenceTimingKey struct{}

// One serial command/heartbeat owns this aggregate. Query goroutines write only their
// private timestamps; the caller merges after done closes, never after timeout.
type sourcePresenceTiming struct {
	wait, native time.Duration
	measured     bool
}

type authorityQueryTiming struct{ started, finished time.Time }

func (p *sourcePresenceTiming) observe(admitted time.Time, query *authorityQueryTiming) {
	if query == nil {
		return
	}
	native := max(0, query.finished.Sub(query.started))
	p.native += native
	p.wait += max(0, time.Since(admitted)-native)
	p.measured = true
}

func (p *sourcePresenceTiming) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, sourcePresenceTimingKey{}, p)
}

func (p *sourcePresenceTiming) detail() handoff.CaptureAuthorityProfile {
	if p == nil || !p.measured || p.wait < 0 || p.native < 0 || p.wait > handoff.Window || p.native > handoff.Window {
		return handoff.CaptureAuthorityProfile{}
	}
	d := handoff.CaptureAuthorityProfile{Measured: true, WaitMicros: uint32(p.wait.Microseconds()), NativeMicros: uint32(p.native.Microseconds())}
	if !d.Valid() {
		return handoff.CaptureAuthorityProfile{}
	}
	return d
}
