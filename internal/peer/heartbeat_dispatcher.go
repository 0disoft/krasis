package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

var ErrUnexpectedHeartbeatFrame = errors.New("unexpected frame on heartbeat dispatcher")

type HeartbeatRuntime interface {
	ObserveHeartbeat(context.Context, string, time.Time) error
	Fail(context.Context, error) error
}

func DispatchHeartbeatFrame(
	ctx context.Context,
	frame transport.Frame,
	observedAt time.Time,
	runtime HeartbeatRuntime,
) error {
	if runtime == nil {
		return errors.New("heartbeat runtime is required")
	}
	if frame.Kind != transport.MessageHeartbeat {
		return ErrUnexpectedHeartbeatFrame
	}
	if observedAt.IsZero() {
		return runtime.Fail(ctx, errors.New("heartbeat observation time is required"))
	}
	sessionID, err := transport.DecodeHeartbeatPayload(frame.Payload)
	if err != nil {
		return runtime.Fail(ctx, err)
	}
	return runtime.ObserveHeartbeat(ctx, sessionID, observedAt)
}
