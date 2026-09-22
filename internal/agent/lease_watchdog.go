package agent

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
)

var ErrLeaseHandshakeExpired = errors.New("lease handshake deadline expired")

func runLeaseWatchdog(ctx context.Context, read func() application.LeaseWatch, policy LivenessPolicy, stop func(error)) error {
	ticker := time.NewTicker(policy.HeartbeatInterval)
	defer ticker.Stop()
	return runLeaseWatchdogTicks(ctx, read, policy, stop, ticker.C, time.Now)
}

func runLeaseWatchdogTicks(ctx context.Context, read func() application.LeaseWatch, policy LivenessPolicy, stop func(error), ticks <-chan time.Time, clock func() time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ticks:
			if !ok {
				return errors.New("lease watchdog ticker stopped unexpectedly")
			}
			// A queued tick describes its scheduled time, not when this worker
			// finally runs after CPU starvation. Never extend the lease by that delay.
			now := clock()
			watch := read()
			budget := policy.PeerTimeout
			cause := peer.ErrPeerLivenessExpired
			switch watch.State {
			case control.StatePending, control.StateInboundPending:
				budget = application.LeaseHandshakeTimeout
				cause = ErrLeaseHandshakeExpired
			case control.StateInboundActive, control.StateOutboundActive:
			default:
				continue
			}
			if !watch.Progress.IsZero() && now.Sub(watch.Progress) >= budget {
				stop(cause)
				return cause
			}
		}
	}
}
