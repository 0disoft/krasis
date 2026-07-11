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

type HeartbeatObservation struct {
	SessionID  string
	ObservedAt time.Time
}

func DispatchHeartbeatFrame(
	ctx context.Context,
	frame transport.Frame,
	observedAt time.Time,
	runtime HeartbeatRuntime,
) (HeartbeatObservation, error) {
	if runtime == nil {
		return HeartbeatObservation{}, errors.New("heartbeat runtime is required")
	}
	if frame.Kind != transport.MessageHeartbeat {
		return HeartbeatObservation{}, ErrUnexpectedHeartbeatFrame
	}
	if observedAt.IsZero() {
		return HeartbeatObservation{}, runtime.Fail(ctx, errors.New("heartbeat observation time is required"))
	}
	sessionID, err := transport.DecodeHeartbeatPayload(frame.Payload)
	if err != nil {
		return HeartbeatObservation{}, runtime.Fail(ctx, err)
	}
	if err := runtime.ObserveHeartbeat(ctx, sessionID, observedAt); err != nil {
		return HeartbeatObservation{}, err
	}
	return HeartbeatObservation{SessionID: sessionID, ObservedAt: observedAt}, nil
}
