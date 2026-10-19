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
	observer peer.LivenessObserver,
) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	ticker := time.NewTicker(policy.HeartbeatInterval)
	defer ticker.Stop()
	return runLivenessTicks(ctx, coordinator, runtime, policy, now, observer, ticker.C)
}

func runLivenessTicks(
	ctx context.Context,
	coordinator *peer.SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	policy LivenessPolicy,
	now peer.ObservationClock,
	observer peer.LivenessObserver,
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
		case scheduled, ok := <-ticks:
			if !ok {
				return errors.New("liveness ticker stopped unexpectedly")
			}
			if timing, ok := observer.(peer.TimerObserver); ok {
				timing.ObserveTimerLateness(max(0, time.Since(scheduled)))
			}
			progress, _ := observer.(peer.HeartbeatSendObserver)
			pulse, err := peer.PulseLivenessObserved(ctx, coordinator, runtime, now, policy.PeerTimeout, progress)
			if err != nil {
				return err
			}
			if pulse.Sent && observer != nil {
				observer.ObserveHeartbeatSent(pulse.SessionID, pulse.ObservedAt)
			}
		}
	}
}
