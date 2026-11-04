//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sys/windows"
)

// A clean retirement receipt precedes the service's final report/handle join.
// This is a receive-only reacquisition budget, never an input/query deadline.
const sourceRebindTimeout = sourceStandbyTimeout

func connectSourceRebind(ctx context.Context, retired bool, connect func(context.Context) (*Channel, error), observe func(string)) (*Channel, error) {
	if !retired {
		return connect(ctx)
	}
	run, cancel := context.WithTimeout(ctx, sourceRebindTimeout)
	defer cancel()
	delay := 50 * time.Millisecond
	var last error
	retried := false
	for {
		if err := run.Err(); err != nil {
			return nil, errors.Join(last, err)
		}
		channel, err := connect(run)
		if err == nil {
			if channel == nil {
				return nil, ErrProtocol
			}
			if err := run.Err(); err != nil {
				return nil, closeSourceChannelResult(channel, err)
			}
			if retried && observe != nil {
				observe("installed_source_rebind_ready")
			}
			return channel, nil
		}
		// No lease bytes were written: retry only a known transient open.
		// Authentication, native denial and joined/unknown causes stay fatal.
		if channel != nil {
			return nil, closeSourceChannelResult(channel, err)
		}
		if !sourceRebindOpenTransient(err) {
			return nil, err
		}
		last = err
		if !retried && observe != nil {
			observe("installed_source_rebind_wait")
		}
		retried = true
		timer := time.NewTimer(delay)
		select {
		case <-run.Done():
			timer.Stop()
			return nil, errors.Join(last, run.Err())
		case <-timer.C:
		}
		delay = min(2*delay, 200*time.Millisecond)
	}
}

func sourceRebindOpenTransient(err error) bool {
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return sourceRebindOpenTransient(wrapped.Unwrap())
	}
	failure, ok := err.(*connectionFailure)
	if !ok || failure.stage != "pipe_open" {
		return false
	}
	return failure.cause == context.DeadlineExceeded || failure.cause == windows.ERROR_FILE_NOT_FOUND || failure.cause == windows.ERROR_PIPE_BUSY
}
