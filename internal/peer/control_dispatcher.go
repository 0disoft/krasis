package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

var ErrUnexpectedControlFrame = errors.New("unexpected frame on control dispatcher")

type LeaseRequestHandler interface {
	HandleLeaseRequest(context.Context, transport.ControlCommand, time.Time) error
}

func DispatchControlFrame(
	ctx context.Context,
	frame transport.Frame,
	observedAt time.Time,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
) error {
	if runtime == nil {
		return errors.New("control runtime is required")
	}
	if frame.Kind != transport.MessageControl {
		return ErrUnexpectedControlFrame
	}
	if observedAt.IsZero() {
		return runtime.Fail(ctx, errors.New("control observation time is required"))
	}
	command, err := transport.DecodeControlPayload(frame.Payload)
	if err != nil {
		return runtime.Fail(ctx, err)
	}
	switch command.Kind {
	case transport.ControlLeaseRequest:
		if requests == nil {
			return runtime.Fail(ctx, errors.New("lease request handler is required"))
		}
		if err := requests.HandleLeaseRequest(ctx, command, observedAt); err != nil {
			return runtime.Fail(ctx, err)
		}
		return nil
	case transport.ControlLeaseGrant:
		_, err = runtime.GrantOutbound(ctx, command.SessionID, command.LeaseID, command.Epoch, observedAt)
	case transport.ControlLeaseReject:
		_, err = runtime.RejectOutbound(ctx, command.SessionID, command.Epoch)
	case transport.ControlLeaseRevoke:
		_, err = runtime.RevokePeer(ctx, command.SessionID, command.LeaseID, command.Epoch)
	default:
		err = errors.New("decoded unsupported control command")
	}
	return err
}
