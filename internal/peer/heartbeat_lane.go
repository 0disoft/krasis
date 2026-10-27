package peer

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

func heartbeatReadLane(ctx context.Context, runtime *application.SessionRuntime, now ObservationClock, timeout time.Duration, observer LivenessObserver) func(transport.Frame) (bool, error) {
	return func(frame transport.Frame) (bool, error) {
		sessionID, err := transport.DecodeHeartbeatPayload(frame.Payload)
		if err != nil {
			return false, nil
		} // Ordered dispatch retains malformed-frame recovery.
		observedAt := now()
		accepted, err := runtime.ObserveHeartbeatAhead(ctx, sessionID, observedAt, timeout)
		if err != nil {
			return false, nil // The ordered owner rechecks deadlines and performs recovery.
		}
		if accepted && observer != nil {
			observer.ObserveHeartbeatReceived(sessionID, observedAt)
			if progress, ok := observer.(HeartbeatReceiveObserver); ok {
				at := time.Now()
				progress.ObserveHeartbeatReceive(HeartbeatStart, at)
				progress.ObserveHeartbeatReceive(HeartbeatGate, at)
				progress.ObserveHeartbeatReceive(HeartbeatWork, at)
				progress.ObserveHeartbeatReceive(HeartbeatComplete, at)
			}
		}
		return accepted, nil
	}
}
