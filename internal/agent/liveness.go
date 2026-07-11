package agent

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/peer"
)

type LivenessPolicy struct {
	HeartbeatInterval time.Duration
	PeerTimeout       time.Duration
}

func (policy LivenessPolicy) Validate() error {
	if policy.HeartbeatInterval <= 0 || policy.PeerTimeout <= policy.HeartbeatInterval {
		return errors.New("heartbeat interval must be positive and peer timeout must be longer")
	}
	return nil
}

func runLiveness(
	ctx context.Context,
	coordinator *peer.SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	policy LivenessPolicy,
	now peer.ObservationClock,
) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	ticker := time.NewTicker(policy.HeartbeatInterval)
	defer ticker.Stop()
	return runLivenessTicks(ctx, coordinator, runtime, policy, now, ticker.C)
}

func runLivenessTicks(
	ctx context.Context,
	coordinator *peer.SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	policy LivenessPolicy,
	now peer.ObservationClock,
	ticks <-chan time.Time,
) error {
	if ctx == nil || coordinator == nil || runtime == nil || now == nil || ticks == nil {
		return errors.New("liveness context, coordinator, runtime, observation clock, and ticks are required")
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-ticks:
			if !ok {
				return errors.New("liveness ticker stopped unexpectedly")
			}
			if _, err := peer.PulseLiveness(ctx, coordinator, runtime, now, policy.PeerTimeout); err != nil {
				return err
			}
		}
	}
}
