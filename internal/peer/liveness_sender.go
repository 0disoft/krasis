package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

var ErrPeerLivenessExpired = errors.New("peer liveness expired")

// PulseLiveness acquires the semantic write operation before reading the
// observation clock, then checks the deadline and writes one heartbeat only
// while the exact session remains active.
func PulseLiveness(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	now ObservationClock,
	timeout time.Duration,
) (LivenessPulse, error) {
	return PulseLivenessObserved(ctx, coordinator, runtime, now, timeout, nil)
}

func PulseLivenessObserved(ctx context.Context, coordinator *SessionWriteCoordinator, runtime *application.SessionRuntime, now ObservationClock, timeout time.Duration, observer HeartbeatSendObserver) (LivenessPulse, error) {
	if coordinator == nil || runtime == nil || now == nil {
		return LivenessPulse{}, errors.New("write coordinator, runtime, and observation clock are required")
	}
	pulse := LivenessPulse{}
	terminal := false
	observe := func(stage HeartbeatStage) {
		if stage >= HeartbeatComplete {
			if terminal {
				return
			}
			terminal = true
		}
		if observer != nil {
			observer.ObserveHeartbeatSend(stage, time.Now())
		}
	}
	observe(HeartbeatStart)
	err := coordinator.Execute(ctx, func(writer FrameWriter) error {
		observe(HeartbeatGate)
		// Input retains its own ordered owner, but a healthy heartbeat reads
		// only the immutable lease proof. Semantic control writes still own
		// this coordinator, so no heartbeat overtakes a revoke/grant write.
		watch, active, observedAt, err := runtime.HeartbeatLease(now, timeout)
		if !active {
			return nil
		}
		pulse.Active, pulse.SessionID, pulse.ObservedAt = true, watch.SessionID, observedAt
		if err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
			defer cancel()
			faulted, checkErr := runtime.CheckLivenessNow(cleanupCtx, now, timeout)
			if checkErr != nil {
				return checkErr
			}
			if faulted {
				return ErrPeerLivenessExpired
			}
		}
		payload, err := transport.EncodeHeartbeatPayload(watch.SessionID)
		if err != nil {
			return failRuntime(ctx, runtime, err)
		}
		observe(HeartbeatWork)
		if err := writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageHeartbeat, Payload: payload}); err != nil {
			observe(HeartbeatFailed)
			return failRuntime(ctx, runtime, err)
		}
		pulse.Sent = true
		observe(HeartbeatComplete)
		return nil
	})
	if err != nil && !pulse.Sent {
		observe(HeartbeatFailed)
	} else if !pulse.Active {
		observe(HeartbeatSkipped)
	}
	return pulse, err
}
