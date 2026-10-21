//go:build windows

package inputworker

import (
	"context"
	"time"
)

// One existing write budget covers authorization, gate admission and bytes.
// Completed outcomes and joined retirement may report after run cancellation;
// they still have a fresh one-second deadline and cannot grant new authority.
func (w *sourceWriter) operation(ctx context.Context, outcome bool) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = w.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if outcome {
		ctx = context.WithoutCancel(ctx)
	}
	deadline := time.Now().Add(time.Second)
	if !outcome && w.ctx != nil {
		if ownerDeadline, ok := w.ctx.Deadline(); ok && ownerDeadline.Before(deadline) {
			deadline = ownerDeadline
		}
	}
	op, cancel := context.WithDeadline(ctx, deadline)
	if !outcome && w.ctx != nil {
		if w.ctx.Err() != nil {
			cancel()
			return op, cancel
		}
		stop := context.AfterFunc(w.ctx, cancel)
		return op, func() { stop(); cancel() }
	}
	return op, cancel
}

// A full proof belongs only to immediate gate admission. Any intervening writer
// invalidates it; waiting never carries an old proof into a later write.
func (w *sourceWriter) lockAuthorized(ctx context.Context, advisory bool) error {
	for {
		check := w.channel.authorizeSourceInput
		if advisory {
			check = w.channel.authorizeSourceAdvisory
		}
		if err := check(ctx); err != nil {
			return err
		}
		if w.mu.TryLock() {
			return nil
		}
		if err := w.mu.LockContext(ctx); err != nil {
			return err
		}
		w.mu.Unlock()
	}
}
