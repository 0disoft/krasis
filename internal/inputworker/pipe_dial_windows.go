//go:build windows

package inputworker

import (
	"context"
	"time"

	"golang.org/x/sys/windows"
)

// Missing-name grace is counted from the first open, including any busy wait.
// It never resets per attempt or extends the enclosing authentication deadline.
func dialPipe(ctx context.Context, missingGrace time.Duration, open func() (*pipeConn, error)) (*pipeConn, error) {
	if ctx == nil || open == nil {
		return nil, ErrAuthority
	}
	started := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pipe, err := open()
		if err == nil {
			if err := ctx.Err(); err != nil {
				_ = pipe.Close()
				return nil, err
			}
			return pipe, nil
		}
		wait := 20 * time.Millisecond
		switch err {
		case windows.ERROR_PIPE_BUSY:
			// Preserve the existing caller-bounded busy-pipe wait.
		case windows.ERROR_FILE_NOT_FOUND:
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
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
