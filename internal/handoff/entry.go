package handoff

import (
	"context"
	"sync"
	"time"
)

// Entry records the installed receiver's one cursor preparation transaction.
// It is separate from actual input, so it cannot consume First, Worst or the
// frame histogram. Matching ACK detail uses the existing bounded probe budget.
func Entry(ctx context.Context) (context.Context, func(error)) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil {
		return ctx, func(error) {}
	}
	c := s.collector
	c.mu.Lock()
	if c.stopped.Load() || a.finished || a.entryStarted || a.summary.Role != "receiver" || a.summary.Backend != "installed" || time.Since(c.started) >= Window {
		c.mu.Unlock()
		return ctx, func(error) {}
	}
	a.entryStarted = true
	c.mu.Unlock()
	v := &sample{started: time.Now()}
	x := context.WithValue(ctx, sampleKey{}, v)
	var once sync.Once
	return x, func(err error) {
		once.Do(func() {
			put(&v.vector, Dispatch, time.Since(v.started))
			v.vector.Outcome = Outcome(err)
			c.mu.Lock()
			defer c.mu.Unlock()
			if a.finished || c.stopped.Load() || time.Since(c.started) >= Window {
				return
			}
			copy := v.vector
			a.summary.Entry = &copy
		})
	}
}
