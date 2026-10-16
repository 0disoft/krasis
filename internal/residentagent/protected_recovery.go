package residentagent

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/inputworker"
)

const protectedRecoveryDelay = 250 * time.Millisecond

// A protected desktop releases the current input session, but a clean release
// keeps the user's connection request alive. The peer observes EOF and retries;
// a manual disconnect still sends the explicit stop frame.
func (operations *Operations) runWithProtectedRecovery(ctx context.Context, alias string, observer *sessionObserver, protectedActive func() bool, wait func(context.Context, time.Duration) error) error {
	defer func() {
		operations.mu.Lock()
		defer operations.mu.Unlock()
		if operations.sessionDone == observer.done {
			operations.sessionTransition = nil
			operations.sessionSourceRefresh = nil
			operations.desktopChanging = false
		}
	}()
	for {
		// Only this runner's capture, transport and input lease are cancelled.
		// The process/local API and the user's connection intent remain alive.
		attempt, stop := context.WithCancelCause(ctx)
		attempt = inputworker.WithDesktopTransitionDrain(attempt, ctx)
		refresh := &sourceRefreshSlot{}
		attempt = context.WithValue(attempt, sourceRefreshKey{}, refresh)
		operations.mu.Lock()
		operations.sessionTransition = stop
		operations.sessionSourceRefresh = refresh
		operations.desktopChanging = false
		operations.mu.Unlock()
		err := operations.runner(attempt, alias, observer)
		cause := context.Cause(attempt)
		stop(context.Canceled)
		var transition *agentlifecycle.TransitionError
		desktopChanged := errors.As(cause, &transition) && cancellationOnly(err, cause)
		// A freshness timeout is not a revocation verdict. Once this exact
		// runner has joined all native owners, preserve the user's connection
		// intent and rebuild from a fresh trust/TLS check. Joined cleanup errors
		// or negative trust verdicts must never enter this retry path.
		trustExpired := errors.Is(err, agent.ErrSessionTrustFreshness) && cancellationOnly(err, agent.ErrSessionTrustFreshness)
		if ctx.Err() != nil || (!desktopChanged && !trustExpired && !protectedInputOnly(err)) {
			return err
		}
		operations.mu.Lock()
		if operations.sessionDone != observer.done || operations.sessionFaulted || operations.manualStop || operations.root.Err() != nil {
			operations.mu.Unlock()
			return err
		}
		observer.retrying = true
		operations.sessionState = "RECONNECTING"
		code := "protected_input_reconnecting"
		if trustExpired {
			code = "session_trust_reconnecting"
		}
		operations.recordLocked(code)
		operations.mu.Unlock()

		// Portable recovery waits until the protected UI has gone. A verified
		// installed desktop transition can rebuild the receive-only session
		// while it remains open. Both paths avoid a tight retry loop.
		for {
			if waitErr := wait(ctx, protectedRecoveryDelay); waitErr != nil {
				return errors.Join(waitErr, context.Cause(ctx))
			}
			if trustExpired {
				// An uninterruptible read can outlive cancellation. Wait for its
				// existing global slot; do not spawn overlapping filesystem reads
				// or use a late result to authorize the replacement connection.
				select {
				case trustReadSlot <- struct{}{}:
					<-trustReadSlot
				default:
					continue
				}
				break
			}
			if desktopChanged || !protectedActive() {
				break
			}
		}
	}
}

func (operations *Operations) handleProtectedTransition(event agentlifecycle.Event) bool {
	if event != agentlifecycle.EventSessionLock && event != agentlifecycle.EventDesktopSwitch {
		return false
	}
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.root.Err() != nil || operations.sessionDone == nil || operations.sessionTransition == nil ||
		operations.inputBackend != "PROTECTED" || operations.sessionFaulted || operations.manualStop || operations.updateApplying || (operations.sessionState == "DISCONNECTING" && !operations.desktopChanging) {
		return false
	}
	operations.recordLocked("lifecycle_" + string(event))
	refreshed, refusal := operations.sessionSourceRefresh.handleObserved(event)
	if refreshed {
		operations.recordLocked("installed_source_refresh_requested")
		return true
	}
	operations.recordLocked(refusal.code())
	operations.desktopChanging = true
	operations.sessionTransition(&agentlifecycle.TransitionError{Event: event})
	return true
}
