package peer

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/transport"
)

type SessionConnection interface {
	ReadFrame() (transport.Frame, error)
	Close() error
}

type ObservationClock func() time.Time

var (
	ErrPeerControlDispatch     = errors.New("peer control frame dispatch failed")
	ErrPeerInputDispatch       = errors.New("peer input frame dispatch failed")
	ErrPeerHeartbeatDispatch   = errors.New("peer heartbeat frame dispatch failed")
	ErrPeerDisconnectRequested = errors.New("peer requested disconnect")
)

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

	progress, _ := observer.(SessionProgressObserver)
	coordinatorProgress, _ := observer.(DispatchCoordinatorObserver)
	heartbeatProgress, _ := observer.(HeartbeatReceiveObserver)
	var previousKind transport.MessageKind
	var previousDispatchStarted, previousDispatchCompleted time.Time
	for {
		readStarted := time.Now()
		handoff.ReadProgress(ctx, handoff.ReadWaiting, readStarted, false)
		if progress != nil {
			progress.ObserveReadStarted(readStarted)
		}
		frame, err := ownedConnection.ReadFrame()
		if err != nil {
			_ = ownedConnection.Close()
			if ctxErr := ctx.Err(); ctxErr != nil {
				ctxErr = errors.Join(ctxErr, abortPendingLease(ctx, requests))
				return failSessionWithLifecycle(ctx, coordinator, runtime, outbound, ctxErr)
			}
			err = classifyPeerReadFailure(connection, err)
			err = errors.Join(err, abortPendingLease(ctx, requests))
			return failSessionWithLifecycle(ctx, coordinator, runtime, outbound, err)
		}
		controlCtx := ctx
		readCompleted := time.Now()
		if frame.Kind == transport.MessageHeartbeat && heartbeatProgress != nil {
			heartbeatProgress.ObserveHeartbeatReceive(HeartbeatStart, readCompleted)
		}
		handoff.ReadProgress(ctx, handoff.ReadBetweenFrames, readCompleted, true)
		if frame.Kind == transport.MessageControl {
			controlCtx = handoff.WithControlRead(ctx, readCompleted)
			controlCtx = handoff.WithReadPredecessor(controlCtx, uint8(previousKind), previousDispatchStarted, previousDispatchCompleted, readStarted)
		}
		if progress != nil {
			progress.ObserveReadCompleted(readCompleted)
		}
		dispatchStarted := time.Now()
		handoff.ReadProgress(ctx, handoff.ReadDispatch, dispatchStarted, false)
		if progress != nil {
			progress.ObserveDispatchStarted(frame.Kind, dispatchStarted)
		}
		var observedAt time.Time
		var heartbeat HeartbeatObservation
		switch frame.Kind {
		case transport.MessageDisconnect:
			if len(frame.Payload) != 0 {
				err = transport.ErrInvalidFrame
			} else {
				err = ErrPeerDisconnectRequested
			}
		case transport.MessageControl:
			observedAt = now()
			err = DispatchControlFrameWithLifecycle(controlCtx, frame, observedAt, runtime, requests, outbound)
			if err != nil {
				err = errors.Join(ErrPeerControlDispatch, err)
			}
		case transport.MessageInput:
			inputCtx, complete := handoff.Input(ctx, "receiver")
			inputCtx, stopInput := context.WithTimeout(inputCtx, application.InputInjectionTimeout)
			queued := time.Now()
			recorded := false
			if coordinatorProgress != nil {
				coordinatorProgress.ObserveDispatchCoordinatorWaiting(queued)
			}
			err = coordinateDispatch(inputCtx, coordinator, func() (dispatchErr error) {
				if coordinatorProgress != nil {
					coordinatorProgress.ObserveDispatchCoordinatorAcquired(time.Now())
				}
				defer func() { complete(dispatchErr); recorded = true }()
				handoff.Record(inputCtx, handoff.InputCoordinator, time.Since(queued))
				return DispatchInputFrame(inputCtx, frame, observedInputRuntime{runtime, now, peerTimeout})
			})
			if !recorded {
				complete(err)
			}
			stopInput()
			if err != nil {
				err = errors.Join(ErrPeerInputDispatch, err)
			}
		case transport.MessageHeartbeat:
			if heartbeatProgress != nil {
				heartbeatProgress.ObserveHeartbeatReceive(HeartbeatGate, time.Now())
			}
			if coordinatorProgress != nil {
				coordinatorProgress.ObserveDispatchCoordinatorWaiting(time.Now())
			}
			err = coordinateDispatch(ctx, coordinator, func() error {
				if heartbeatProgress != nil {
					heartbeatProgress.ObserveHeartbeatReceive(HeartbeatWork, time.Now())
				}
				if coordinatorProgress != nil {
					coordinatorProgress.ObserveDispatchCoordinatorAcquired(time.Now())
				}
				observedAt = now()
				// After normal return, the dispatcher must still validate and
				// drain the exact retired heartbeat (ADR 0020). Only active
				// leases have a deadline; other identities remain rejected.
				cleanupCtx, cancel := context.WithTimeout(ctx, CleanupTimeout)
				defer cancel()
				snapshot, snapshotErr := runtime.SnapshotContext(cleanupCtx)
				if snapshotErr != nil {
					return snapshotErr
				}
				state := snapshot.Control.State
				if peerTimeout > 0 && (state == control.StateInboundActive || state == control.StateOutboundActive) {
					faulted, livenessErr := runtime.CheckLiveness(cleanupCtx, observedAt, peerTimeout)
					if livenessErr != nil {
						return livenessErr
					}
					if faulted {
						return ErrPeerLivenessExpired
					}
				}
				var dispatchErr error
				heartbeat, dispatchErr = DispatchHeartbeatFrame(cleanupCtx, frame, observedAt, runtime)
				return dispatchErr
			})
			if heartbeatProgress != nil {
				stage := HeartbeatComplete
				if err != nil {
					stage = HeartbeatFailed
				}
				heartbeatProgress.ObserveHeartbeatReceive(stage, time.Now())
			}
			if err != nil {
				err = errors.Join(ErrPeerHeartbeatDispatch, err)
			}
		default:
			err = transport.ErrUnsupportedFrame
		}
		dispatchCompleted := time.Now()
		previousKind, previousDispatchStarted, previousDispatchCompleted = frame.Kind, dispatchStarted, dispatchCompleted
		handoff.ReadProgress(ctx, handoff.ReadBetweenFrames, dispatchCompleted, false)
		if progress != nil {
			progress.ObserveDispatchCompleted(dispatchCompleted)
		}
		if err != nil {
			_ = ownedConnection.Close()
			err = errors.Join(err, abortPendingLease(ctx, requests))
			return failSessionWithLifecycle(ctx, coordinator, runtime, outbound, err)
		}
		if frame.Kind == transport.MessageHeartbeat && observer != nil {
			observer.ObserveHeartbeatReceived(heartbeat.SessionID, heartbeat.ObservedAt)
		}
	}
}

type pendingLeaseAborter interface {
	AbortPending(context.Context) error
}

func abortPendingLease(ctx context.Context, requests LeaseRequestHandler) error {
	aborter, ok := requests.(pendingLeaseAborter)
	if !ok {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
	defer cancel()
	return aborter.AbortPending(cleanupCtx)
}

func classifyPeerReadFailure(connection SessionConnection, readErr error) error {
	if state, ok := connection.(localInterruptState); ok && state.LocalInterruptObserved() {
		state.ObserveReadAfterLocalInterrupt()
		return errors.Join(ErrPeerReadAfterLocalInterrupt, state.LocalInterruptCause(), readErr)
	}
	if errors.Is(readErr, io.EOF) && !errors.Is(readErr, transport.ErrInvalidFrame) {
		return errors.Join(ErrPeerRemoteEOF, io.ErrUnexpectedEOF)
	}
	if errors.Is(readErr, io.ErrUnexpectedEOF) || errors.Is(readErr, transport.ErrFramePayloadTruncated) {
		return errors.Join(ErrPeerFrameTruncated, readErr)
	}
	return errors.Join(ErrPeerReadFailure, readErr)
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
