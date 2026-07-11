package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
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
	if coordinator == nil || runtime == nil || now == nil {
		return LivenessPulse{}, errors.New("write coordinator, runtime, and observation clock are required")
	}
	pulse := LivenessPulse{}
	err := coordinator.Execute(ctx, func(writer FrameWriter) error {
		snapshot := runtime.Snapshot()
		if snapshot.Control.State != control.StateInboundActive && snapshot.Control.State != control.StateOutboundActive {
			return nil
		}
		pulse.Active = true
		pulse.SessionID = snapshot.Control.SessionID
		pulse.ObservedAt = now()
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
		defer cancel()
		faulted, err := runtime.CheckLiveness(cleanupCtx, pulse.ObservedAt, timeout)
		if err != nil {
			return err
		}
		if faulted {
			return ErrPeerLivenessExpired
		}
		payload, err := transport.EncodeHeartbeatPayload(snapshot.Control.SessionID)
		if err != nil {
			return failRuntime(ctx, runtime, err)
		}
		if err := writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageHeartbeat, Payload: payload}); err != nil {
			return failRuntime(ctx, runtime, err)
		}
		pulse.Sent = true
		return nil
	})
	return pulse, err
}
