package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/agentlifecycle"
)

type transitionDrainKey struct{}

// WithDesktopTransitionDrain binds the installed runner's soft cancellation to
// its independent connection-intent lifetime. Only verified desktop/lock events
// may finish a fully written input request; shutdown and manual stop still abort.
// It does not authorize input, renew a lease, or permit another write.
func WithDesktopTransitionDrain(ctx, hardStop context.Context) context.Context {
	return context.WithValue(ctx, transitionDrainKey{}, hardStop)
}

func transitionDrainStop(ctx context.Context) context.Context {
	hardStop, _ := ctx.Value(transitionDrainKey{}).(context.Context)
	var transition *agentlifecycle.TransitionError
	if hardStop == nil || hardStop.Err() != nil || !errors.As(context.Cause(ctx), &transition) || transition == nil {
		return nil
	}
	if transition.Event != agentlifecycle.EventDesktopSwitch && transition.Event != agentlifecycle.EventSessionLock {
		return nil
	}
	return hardStop
}

// The cancellation callback is joined before the next request can acquire the
// client mutex. A late callback cannot close a later request's channel.
func requestCancellation(ctx context.Context, channel *Channel, deadline time.Time, input bool, submitted *atomic.Bool) func() {
	finished, joined := make(chan struct{}), make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(joined)
		if hardStop := transitionDrainStop(ctx); input && submitted.Load() && hardStop != nil {
			timer := time.NewTimer(time.Until(deadline))
			defer timer.Stop()
			select {
			case <-finished:
				return
			case <-hardStop.Done():
			case <-timer.C:
			}
		}
		_ = channel.Close()
	})
	return func() {
		close(finished)
		if !stop() {
			<-joined
		}
	}
}
