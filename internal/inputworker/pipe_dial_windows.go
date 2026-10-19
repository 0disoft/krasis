//go:build windows

package inputworker

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
)

// Newly rebinding service endpoints are latency-sensitive. Keep the fast retry
// window bounded; a slow/missing service returns to the original 20ms cadence.
func pipeRetryDelay(elapsed time.Duration) time.Duration {
	if elapsed < 16*time.Millisecond {
		return 2 * time.Millisecond
	}
	if elapsed < 100*time.Millisecond {
		return 5 * time.Millisecond
	}
	return 20 * time.Millisecond
}

// Missing-name grace is counted from the first open, including any busy wait.
// It never resets per attempt or extends the enclosing authentication deadline.
func dialPipe(ctx context.Context, missingGrace time.Duration, open func() (*pipeConn, error)) (*pipeConn, error) {
	if ctx == nil || open == nil {
		return nil, ErrAuthority
	}
	started := time.Now()
	profile := handoff.PipeOpenProfile{Schema: 1}
	defer func() { handoff.RecordPipeOpen(ctx, profile) }()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		openStarted := time.Now()
		pipe, err := open()
		profile.Attempts++
		profile.OpenMicros += uint32(min(time.Since(openStarted), handoff.Window).Microseconds())
		if err == nil {
			if err := ctx.Err(); err != nil {
				_ = pipe.Close()
				return nil, err
			}
			return pipe, nil
		}
		wait := pipeRetryDelay(time.Since(started))
		switch err {
		case windows.ERROR_PIPE_BUSY:
			profile.Busy++
			// Preserve the existing caller-bounded busy-pipe wait.
		case windows.ERROR_FILE_NOT_FOUND:
			profile.Missing++
			remaining := missingGrace - time.Since(started)
			if missingGrace <= 0 || remaining <= 0 {
				return nil, err
			}
			if remaining < wait {
				wait = remaining
			}
		default:
			// Access denial, invalid names, broken streams and authentication
			// failures are never made retryable by the missing-name grace.
			return nil, err
		}
		waitStarted := time.Now()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			profile.WaitMicros += uint32(min(time.Since(waitStarted), handoff.Window).Microseconds())
			return nil, ctx.Err()
		case <-timer.C:
			profile.WaitMicros += uint32(min(time.Since(waitStarted), handoff.Window).Microseconds())
		}
	}
}
