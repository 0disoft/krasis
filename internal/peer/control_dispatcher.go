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
	return dispatchControlFrame(ctx, frame, observedAt, runtime, requests, nil)
}

func DispatchControlFrameWithLifecycle(
	ctx context.Context,
	frame transport.Frame,
	observedAt time.Time,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
) error {
	return dispatchControlFrame(ctx, frame, observedAt, runtime, requests, outbound)
}

func dispatchControlFrame(
	ctx context.Context,
	frame transport.Frame,
	observedAt time.Time,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
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
		if outbound != nil {
			err = outbound.GrantPeer(ctx, command, observedAt)
		} else {
			_, err = runtime.GrantOutbound(ctx, command.SessionID, command.LeaseID, command.Epoch, observedAt)
		}
	case transport.ControlLeaseReject:
		if outbound != nil {
			err = outbound.RejectPeer(ctx, command)
		} else {
			_, err = runtime.RejectOutbound(ctx, command.SessionID, command.Epoch)
		}
	case transport.ControlLeaseRevoke:
		if outbound != nil {
			err = outbound.RevokePeer(ctx, command)
		} else {
			_, err = runtime.RevokePeer(ctx, command.SessionID, command.LeaseID, command.Epoch)
		}
	default:
		err = errors.New("decoded unsupported control command")
	}
	return err
}
