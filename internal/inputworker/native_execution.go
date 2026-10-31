package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

const nativeProofReplyReserve = 250 * time.Millisecond

var errNativeProofPending = errors.New("native service proof did not finish before reply budget")

// Only the independently authenticated native child supplies these callbacks.
// A timed-out proof permanently fences this serial dispatcher. Its Authority
// still pins handles until the native query returns; exact process/IPC
// retirement remains mandatory before recovery or a replacement worker starts.
func nativeInputExecutor(desktop executor, service func() error, parent func() bool, expires time.Time, releaseOnly bool, timings ...*workerTiming) executor {
	var fenced atomic.Bool
	return func(ctx context.Context, work func() error) error {
		if ctx == nil || work == nil || desktop == nil || service == nil || parent == nil || fenced.Load() {
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
		err := overlapNativeProof(ctx, service, checkedDesktop, work, timing)
		if errors.Is(err, errNativeProofPending) {
			fenced.Store(true)
		}
		return err
	}
}

// Reserve ACK time from the original budget, never after desktop acquisition.
// The one-second query bound also applies without a caller deadline.
func nativeProofDeadline(ctx context.Context) time.Time {
	now := time.Now()
	deadline := now.Add(time.Second)
	if caller, ok := ctx.Deadline(); ok {
		remaining := caller.Sub(now)
		reserve := min(nativeProofReplyReserve, max(0, remaining/4))
		if bound := caller.Add(-reserve); bound.Before(deadline) {
			deadline = bound
		}
	}
	return deadline
}

// Fresh service and desktop proofs overlap. A pending service query cannot
// hold an unapplied command's negative ACK hostage. Its goroutine publishes
// only to its own buffered result slot, never timing memory or an input callback.
func overlapNativeProof(ctx context.Context, service func() error, desktop executor, work func() error, timing *workerTiming) error {
	if timing != nil {
		*timing = workerTiming{}
	}
	proofDeadline := nativeProofDeadline(ctx)
	type proofResult struct {
		err     error
		elapsed time.Duration
	}
	done := make(chan proofResult, 1)
	go func() {
		started := time.Now()
		result := proofResult{}
		defer func() {
			if recover() != nil {
				result.err = errors.Join(ErrAuthority, inputdesktop.ErrCallbackPanic)
			}
			result.elapsed = time.Since(started)
			done <- result
		}()
		result.err = service()
	}()
	var proof proofResult
	joined := false
	var scopeElapsed, applied time.Duration
	defer func() {
		if timing != nil {
			if joined {
				timing.record(0, proof.elapsed)
			}
			timing.record(1, scopeElapsed-applied)
		}
	}()
	deferred, entered := false, false
	run := func() error {
		entered = true
		if !joined {
			select {
			case proof = <-done:
				joined = true
			default:
				deferred = true
				return nil // Discard this desktop check without attempting input.
			}
		}
		if proof.err != nil {
			return proof.err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !time.Now().Before(proofDeadline) {
			return context.DeadlineExceeded
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
	if !joined {
		proofCtx, cancel := context.WithDeadline(ctx, proofDeadline)
		defer cancel()
		select {
		case proof = <-done:
			joined = true
		case <-proofCtx.Done():
			return errors.Join(err, ErrAuthority, errNativeProofPending, proofCtx.Err())
		}
	}
	if err != nil || proof.err != nil {
		return errors.Join(err, proof.err)
	}
	if !entered {
		return ErrAuthority
	}
	if deferred {
		entered = false
		started = time.Now()
		err = desktop(ctx, run) // Fresh agent/desktop after the late service proof.
		scopeElapsed += time.Since(started)
		if err == nil && !entered {
			return ErrAuthority
		}
	}
	return err
}
