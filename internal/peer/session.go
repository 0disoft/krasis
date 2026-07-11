package peer

import (
	"context"
	"errors"
	"io"
	"sync"
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
	return runSession(ctx, connection, nil, runtime, requests, nil, now, 0, nil)
}

func RunSessionWithLifecycle(
	ctx context.Context,
	connection SessionConnection,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
	now ObservationClock,
) error {
	if coordinator == nil {
		return errors.New("lifecycle session write coordinator is required")
	}
	return runSession(ctx, connection, coordinator, runtime, requests, outbound, now, 0, nil)
}

func RunSessionWithLifecycleObserved(
	ctx context.Context,
	connection SessionConnection,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
	now ObservationClock,
	peerTimeout time.Duration,
	observer LivenessObserver,
) error {
	if coordinator == nil {
		return errors.New("lifecycle session write coordinator is required")
	}
	if peerTimeout < 0 {
		return errors.New("peer timeout cannot be negative")
	}
	return runSession(ctx, connection, coordinator, runtime, requests, outbound, now, peerTimeout, observer)
}

func runSession(
	ctx context.Context,
	connection SessionConnection,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	requests LeaseRequestHandler,
	outbound OutboundLifecycle,
	now ObservationClock,
	peerTimeout time.Duration,
	observer LivenessObserver,
) (returnErr error) {
	if connection == nil || runtime == nil || now == nil {
		return errors.New("session connection, runtime, and observation clock are required")
	}
	ownedConnection := &sessionConnectionOwner{SessionConnection: connection}
	stopCancelClose := context.AfterFunc(ctx, func() {
		_ = ownedConnection.Close()
	})
	defer func() {
		stopCancelClose()
		returnErr = errors.Join(returnErr, ownedConnection.Close())
	}()

	for {
		frame, err := ownedConnection.ReadFrame()
		if err != nil {
			_ = ownedConnection.Close()
			if ctxErr := ctx.Err(); ctxErr != nil {
				return failSessionWithLifecycle(ctx, coordinator, runtime, outbound, ctxErr)
			}
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return failSessionWithLifecycle(ctx, coordinator, runtime, outbound, err)
		}
		var observedAt time.Time
		var heartbeat HeartbeatObservation
		switch frame.Kind {
		case transport.MessageControl:
			observedAt = now()
			err = DispatchControlFrameWithLifecycle(ctx, frame, observedAt, runtime, requests, outbound)
		case transport.MessageInput:
			err = coordinateDispatch(ctx, coordinator, func() error {
				return DispatchInputFrame(ctx, frame, runtime)
			})
		case transport.MessageHeartbeat:
			err = coordinateDispatch(ctx, coordinator, func() error {
				observedAt = now()
				if peerTimeout > 0 {
					cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
					defer cancel()
					faulted, livenessErr := runtime.CheckLiveness(cleanupCtx, observedAt, peerTimeout)
					if livenessErr != nil {
						return livenessErr
					}
					if faulted {
						return ErrPeerLivenessExpired
					}
				}
				var dispatchErr error
				heartbeat, dispatchErr = DispatchHeartbeatFrame(ctx, frame, observedAt, runtime)
				return dispatchErr
			})
		default:
			err = transport.ErrUnsupportedFrame
		}
		if err != nil {
			_ = ownedConnection.Close()
			return failSessionWithLifecycle(ctx, coordinator, runtime, outbound, err)
		}
		if frame.Kind == transport.MessageHeartbeat && observer != nil {
			observer.ObserveHeartbeatReceived(heartbeat.SessionID, heartbeat.ObservedAt)
		}
	}
}

type sessionConnectionOwner struct {
	SessionConnection
	closeOnce sync.Once
	closeErr  error
}

func (connection *sessionConnectionOwner) Close() error {
	connection.closeOnce.Do(func() {
		connection.closeErr = connection.SessionConnection.Close()
	})
	return connection.closeErr
}

func failSessionWithLifecycle(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
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
	if coordinator != nil {
		return failRuntimeCoordinated(ctx, coordinator, runtime, cause, nil)
	}
	return failSession(ctx, runtime, cause)
}

func coordinateDispatch(ctx context.Context, coordinator *SessionWriteCoordinator, dispatch func() error) error {
	if coordinator == nil {
		return dispatch()
	}
	return coordinator.Execute(ctx, func(FrameWriter) error { return dispatch() })
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
