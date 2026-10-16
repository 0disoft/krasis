package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/truststore"
)

type trustReadObservation struct {
	serial         uint64
	completed      trustCheckCompletion
	completedTrace *truststore.ReadTrace
	validatedAt    time.Time
	reading        bool
	started        time.Time
	trace          *truststore.ReadTrace
	deferred       bool
}

type sessionTrustReader struct {
	latest atomic.Pointer[trustReadObservation]
	wake   chan struct{}
}

func deferredTrustRead(err error) bool {
	return errors.Is(err, errTrustReadTimeout) || errors.Is(err, errTrustReadPending) || err == truststore.ErrStoreBusy
}

func (r *sessionTrustReader) publish(value trustReadObservation) {
	r.latest.Store(&value)
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// One session-owned reader waits on the actual cadence and performs reads
// serially. Observation/logging cannot postpone the next read or create one
// goroutine per tick. The global slot still bounds uninterruptible OS reads
// across canceled sessions. Every success must finish before the previously
// validated deadline; nothing renews freshness from delivery time.
func startSessionTrustReader(ctx context.Context, ticks <-chan time.Time, deadline time.Time, check func(*truststore.ReadTrace) error, traced bool) *sessionTrustReader {
	r := &sessionTrustReader{wake: make(chan struct{}, 1)}
	go func() {
		var current trustReadObservation
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ticks:
				if !ok || ctx.Err() != nil {
					return
				}
			}
			if !time.Now().Before(deadline) {
				current.serial++
				current.completed = trustCheckCompletion{err: context.DeadlineExceeded, finished: time.Now()}
				r.publish(current)
				return
			}
			select {
			case trustReadSlot <- struct{}{}:
			default:
				current.deferred = true
				r.publish(current)
				continue
			}
			current.reading, current.started, current.trace = true, time.Now(), nil
			if traced {
				current.trace = truststore.NewReadTrace()
			}
			r.publish(current)
			readCtx, stopRead := context.WithDeadline(ctx, deadline)
			err := func() error {
				defer func() { <-trustReadSlot }()
				return checkMonitoredTrustObserved(readCtx, func() error { return check(current.trace) }, agent.Wait, current.trace)
			}()
			finished := time.Now()
			stopRead()
			if ctx.Err() != nil {
				return
			}
			// A cadence tick received during this exact read cannot enqueue
			// another overlapping/immediate retry after a slow filesystem call.
			select {
			case <-ticks:
			default:
			}
			if err == nil {
				if !finished.Before(deadline) {
					err = context.DeadlineExceeded
				} else {
					deadline = finished.Add(trustValidationMaxAge)
					current.validatedAt = finished
				}
			}
			current.serial++
			current.completed = trustCheckCompletion{err: err, finished: finished}
			current.completedTrace = current.trace
			current.reading, current.deferred = false, deferredTrustRead(err)
			r.publish(current)
			if err != nil && !deferredTrustRead(err) {
				return // A revocation/negative verdict is terminal, never overwritten.
			}
		}
	}()
	return r
}
