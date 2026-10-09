package launcherui

import (
	"context"
	"time"
)

// Only the preparation owner mutates its result. A new copy or sharing epoch
// cancels the child; callers join it and discard any late success.
func prepareClipboardFiles(ctx context.Context, current func() bool, prepare func(context.Context) ([]byte, error)) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	stop := cancelClipboardRevision(ctx, cancel, current)
	defer func() { cancel(); stop() }()
	if !current() {
		return nil, context.Canceled
	}
	data, err := prepare(ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !current() {
		return nil, context.Canceled
	}
	return data, err
}

func cancelClipboardRevision(ctx context.Context, cancel context.CancelFunc, current func() bool) func() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTicker(25 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if !current() {
					cancel()
					return
				}
			}
		}
	}()
	return func() { <-done }
}
