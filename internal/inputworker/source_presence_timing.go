package inputworker

import (
	"context"
	"time"
)

type sourcePresenceTimingKey struct{}

// One serial heartbeat owns this aggregate. Query goroutines write only their
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
