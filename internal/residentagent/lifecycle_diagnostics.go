package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/agentlifecycle"
)

// Observe the process owner's cancellation before cleanup finishes. No desktop,
// username or window identifiers cross the diagnostic boundary.
func observeLifecycleStop(ctx context.Context, record func(string)) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		var transition *agentlifecycle.TransitionError
		if errors.As(context.Cause(ctx), &transition) && transition.Event.Validate() == nil {
			record("lifecycle_" + string(transition.Event))
		} else if errors.Is(context.Cause(ctx), agentlifecycle.ErrMonitorStopped) {
			record("lifecycle_monitor_failed")
		}
	}()
	return done
}
