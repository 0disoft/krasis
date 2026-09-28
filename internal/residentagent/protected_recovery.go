package residentagent

import (
	"context"
	"errors"
	"time"
)

const protectedRecoveryDelay = 250 * time.Millisecond

// A protected desktop releases the current input session, but a clean release
// keeps the user's connection request alive. The peer observes EOF and retries;
// a manual disconnect still sends the explicit stop frame.
func (operations *Operations) runWithProtectedRecovery(ctx context.Context, alias string, observer *sessionObserver, protectedActive func() bool, wait func(context.Context, time.Duration) error) error {
	for {
		err := operations.runner(ctx, alias, observer)
		if ctx.Err() != nil || !protectedInputOnly(err) {
			return err
		}
		operations.mu.Lock()
		if operations.sessionDone != observer.done || operations.sessionFaulted || operations.manualStop || operations.root.Err() != nil {
			operations.mu.Unlock()
			return err
		}
		observer.retrying = true
		operations.sessionState = "RECONNECTING"
		operations.recordLocked("protected_input_reconnecting")
		operations.mu.Unlock()

		// Keep local input available until the protected UI has gone. A short
		// delay also prevents a rapidly changing foreground from spinning.
		for {
			if waitErr := wait(ctx, protectedRecoveryDelay); waitErr != nil {
				return errors.Join(waitErr, context.Cause(ctx))
			}
			if !protectedActive() {
				break
			}
		}
	}
}
