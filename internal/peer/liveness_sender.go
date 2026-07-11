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

// PulseLiveness checks the current peer deadline and, while the exact session
// remains active, writes one heartbeat in the same semantic write operation.
func PulseLiveness(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	observedAt time.Time,
	timeout time.Duration,
) (bool, error) {
	if coordinator == nil || runtime == nil {
		return false, errors.New("write coordinator and runtime are required")
	}
	active := false
	err := coordinator.Execute(ctx, func(writer FrameWriter) error {
		snapshot := runtime.Snapshot()
		if snapshot.Control.State != control.StateInboundActive && snapshot.Control.State != control.StateOutboundActive {
			return nil
		}
		active = true
		faulted, err := runtime.CheckLiveness(ctx, observedAt, timeout)
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
		return nil
	})
	return active, err
}
