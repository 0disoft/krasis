package peer

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

var ErrUnexpectedInputFrame = errors.New("unexpected frame on input dispatcher")

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
