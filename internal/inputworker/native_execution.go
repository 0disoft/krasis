package inputworker

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

// desktop must be the native child's authenticated Scope.Do. Its binding
// freshly checks the agent/session; do not repeat that RPC before entering it.
// Fresh service authorization overlaps the independent desktop/agent check.
// If it finishes later, reenter the desktop after joining it: a slow SCM query
// must not leave the selected desktop stale. Parent/lease checks precede input.
func nativeInputExecutor(desktop executor, service func() error, parent func() bool, expires time.Time, releaseOnly bool, timings ...*workerTiming) executor {
	return func(ctx context.Context, work func() error) error {
		if ctx == nil || work == nil || desktop == nil || service == nil || parent == nil {
			return ErrAuthority
		}
		valid := func() bool { return releaseOnly || time.Now().Before(expires) }
		if ctx.Err() != nil || !valid() {
			return ErrAuthority
		}
		checkedDesktop := func(ctx context.Context, operation func() error) error {
			return desktop(ctx, func() error {
				if !parent() || !valid() {
					return ErrAuthority
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				return operation()
			})
		}
		var timing *workerTiming
		if len(timings) == 1 {
			timing = timings[0]
		}
		return overlapNativeProof(ctx, service, checkedDesktop, work, timing)
	}
}

// One serial native dispatcher owns at most one query. Always join it, even
// when desktop acquisition fails, before the caller can close authority pins.
// This is fresh work for this command; neither an idle proof nor a cache.
func overlapNativeProof(ctx context.Context, service func() error, desktop executor, work func() error, timing *workerTiming) error {
	if timing != nil {
		*timing = workerTiming{}
	}
	done := make(chan struct{})
	var serviceErr error
	var serviceElapsed time.Duration
	go func() {
		started := time.Now()
		defer func() {
			// This query used to run inside the desktop callback's panic fence.
			// Keep that fail-closed behavior when moving it to another goroutine.
			if recover() != nil {
				serviceErr = errors.Join(ErrAuthority, inputdesktop.ErrCallbackPanic)
			}
			serviceElapsed = time.Since(started)
			close(done)
		}()
		serviceErr = service()
	}()
	var scopeElapsed, applied time.Duration
	defer func() {
		<-done
		if timing != nil {
			// These proof spans overlap. Apply remains actual native work here;
			// the journal's containing IPC span is recorded by journalExecution.
			timing.record(0, serviceElapsed)
			timing.record(1, scopeElapsed-applied)
		}
	}()
	deferred, entered := false, false
	run := func() error {
		entered = true
		select {
		case <-done:
		default:
			deferred = true
			return nil // No input was attempted; discard this desktop check.
		}
		if serviceErr != nil {
			return serviceErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		err := work()
		applied = time.Since(started)
		if timing != nil {
			timing.record(2, applied)
		}
		return err
	}
	started := time.Now()
	err := desktop(ctx, run)
	scopeElapsed = time.Since(started)
	<-done
	if err != nil || serviceErr != nil {
		return errors.Join(err, serviceErr)
	}
	if !entered {
		return ErrAuthority
	}
	if deferred {
		entered = false
		started = time.Now()
		err = desktop(ctx, run) // Fresh agent/desktop after the slow service query.
		scopeElapsed += time.Since(started)
		if err == nil && !entered {
			return ErrAuthority
		}
	}
	return err
}
