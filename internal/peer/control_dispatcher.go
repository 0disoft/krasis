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

type LeaseCommitHandler interface {
	HandleLeaseCommit(context.Context, transport.ControlCommand, time.Time) error
}

type LeaseAbortHandler interface {
	HandleLeaseAbort(context.Context, transport.ControlCommand) error
}

type InputPreparationHandler interface {
	HandleInputPreparation(context.Context, time.Time)
}

func DispatchControlFrame(
	ctx context.Context,
	frame transport.Frame,
	observedAt time.Time,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
) error {
	return dispatchControlFrame(ctx, frame, observedAt, runtime, requests, nil, false)
}

func DispatchControlFrameWithLifecycle(
	ctx context.Context,
	frame transport.Frame,
	observedAt time.Time,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
) error {
	return dispatchControlFrame(ctx, frame, observedAt, runtime, requests, outbound, true)
}

func dispatchControlFrame(
	ctx context.Context,
	frame transport.Frame,
	observedAt time.Time,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
	deferFailure bool,
) error {
	if runtime == nil {
		return errors.New("control runtime is required")
	}
	if frame.Kind != transport.MessageControl {
		return ErrUnexpectedControlFrame
	}
	if observedAt.IsZero() {
		return failControlDispatch(ctx, runtime, errors.New("control observation time is required"), deferFailure)
	}
	command, err := transport.DecodeControlPayload(frame.Payload)
	if err != nil {
		return failControlDispatch(ctx, runtime, err, deferFailure)
	}
	switch command.Kind {
	case transport.ControlPrepareInput:
		// The hint carries no lease and must never mutate the runtime or
		// block the frame reader on privileged process creation.
		if handler, ok := requests.(InputPreparationHandler); ok {
			handler.HandleInputPreparation(ctx, observedAt)
		}
		return nil
	case transport.ControlLeaseRequest:
		if requests == nil {
			return failControlDispatch(ctx, runtime, errors.New("lease request handler is required"), deferFailure)
		}
		if err := requests.HandleLeaseRequest(ctx, command, observedAt); err != nil {
			return failControlDispatch(ctx, runtime, err, deferFailure)
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
	case transport.ControlLeaseCommit:
		handler, ok := requests.(LeaseCommitHandler)
		if !ok {
			return failControlDispatch(ctx, runtime, errors.New("lease commit handler is required"), deferFailure)
		}
		err = handler.HandleLeaseCommit(ctx, command, observedAt)
	case transport.ControlLeaseAbort:
		handler, ok := requests.(LeaseAbortHandler)
		if !ok {
			return failControlDispatch(ctx, runtime, errors.New("lease abort handler is required"), deferFailure)
		}
		err = handler.HandleLeaseAbort(ctx, command)
	default:
		err = errors.New("decoded unsupported control command")
	}
	return err
}

func failControlDispatch(
	ctx context.Context,
	runtime *application.SessionRuntime,
	cause error,
	deferFailure bool,
) error {
	if deferFailure {
		return cause
	}
	return runtime.Fail(ctx, cause)
}
