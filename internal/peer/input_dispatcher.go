package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

var ErrUnexpectedInputFrame = errors.New("unexpected frame on input dispatcher")

type observedInputRuntime struct {
	*application.SessionRuntime
	now     ObservationClock
	timeout time.Duration
}

func (runtime observedInputRuntime) HandleInputAction(ctx context.Context, envelope control.InputEnvelope, action control.InputAction) error {
	if runtime.timeout == 0 {
		return runtime.SessionRuntime.HandleInputAction(ctx, envelope, action)
	}
	err := runtime.HandleInputActionWithLiveness(ctx, envelope, action, runtime.now, runtime.timeout)
	if errors.Is(err, application.ErrInputLivenessExpired) {
		return errors.Join(ErrPeerLivenessExpired, err)
	}
	return err
}

type InputRuntime interface {
	HandleInputAction(context.Context, control.InputEnvelope, control.InputAction) error
	Fail(context.Context, error) error
}

func DispatchInputFrame(ctx context.Context, frame transport.Frame, runtime InputRuntime) error {
	if runtime == nil {
		return errors.New("input runtime is required")
	}
	if frame.Kind != transport.MessageInput {
		return ErrUnexpectedInputFrame
	}
	envelope, action, err := transport.DecodeInputPayload(frame.Payload)
	if err != nil {
		return runtime.Fail(ctx, err)
	}
	return runtime.HandleInputAction(ctx, envelope, action)
}
