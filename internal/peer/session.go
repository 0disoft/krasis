package peer

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

type SessionConnection interface {
	ReadFrame() (transport.Frame, error)
	Close() error
}

type ObservationClock func() time.Time

func RunSession(
	ctx context.Context,
	connection SessionConnection,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	now ObservationClock,
) error {
	return runSession(ctx, connection, runtime, requests, nil, now)
}

func RunSessionWithLifecycle(
	ctx context.Context,
	connection SessionConnection,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
	now ObservationClock,
) error {
	return runSession(ctx, connection, runtime, requests, outbound, now)
}

func runSession(
	ctx context.Context,
	connection SessionConnection,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
	now ObservationClock,
) error {
	if connection == nil || runtime == nil || now == nil {
		return errors.New("session connection, runtime, and observation clock are required")
	}
	stopCancelClose := context.AfterFunc(ctx, func() {
		_ = connection.Close()
	})
	defer stopCancelClose()
	defer connection.Close()

	for {
		frame, err := connection.ReadFrame()
		if err != nil {
			_ = connection.Close()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return failSessionWithLifecycle(ctx, runtime, outbound, ctxErr)
			}
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return failSessionWithLifecycle(ctx, runtime, outbound, err)
		}
		observedAt := now()
		switch frame.Kind {
		case transport.MessageControl:
			err = DispatchControlFrameWithLifecycle(ctx, frame, observedAt, runtime, requests, outbound)
		case transport.MessageInput:
			err = DispatchInputFrame(ctx, frame, runtime)
		case transport.MessageHeartbeat:
			err = DispatchHeartbeatFrame(ctx, frame, observedAt, runtime)
		default:
			err = transport.ErrUnsupportedFrame
		}
		if err != nil {
			_ = connection.Close()
			return failSessionWithLifecycle(ctx, runtime, outbound, err)
		}
	}
}

func failSessionWithLifecycle(
	ctx context.Context,
	runtime *application.SessionRuntime,
	outbound OutboundLifecycle,
	cause error,
) error {
	if outbound != nil {
		if cause == nil {
			cause = errors.New("peer session terminated")
		}
		return outbound.Fail(ctx, cause)
	}
	return failSession(ctx, runtime, cause)
}

func failSession(ctx context.Context, runtime *application.SessionRuntime, cause error) error {
	if cause == nil {
		cause = errors.New("peer session terminated")
	}
	if err := failRuntime(ctx, runtime, cause); err != nil {
		return err
	}
	return cause
}
