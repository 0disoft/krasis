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
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
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
