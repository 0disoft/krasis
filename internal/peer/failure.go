package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

const CleanupTimeout = 2 * time.Second

var (
	ErrOutboundBegin               = errors.New("outbound boundary admission failed")
	ErrOutboundRequestWrite        = errors.New("outbound lease request write outcome is unknown")
	ErrOutboundGrantApply          = errors.New("outbound lease grant apply failed")
	ErrInboundGrantWrite           = errors.New("inbound lease grant write outcome is unknown")
	ErrInboundRevokeWrite          = errors.New("inbound lease revoke write outcome is unknown")
	ErrPeerReadAfterLocalInterrupt = errors.New("peer read failed after local connection interrupt")
	ErrPeerRemoteEOF               = errors.New("peer closed the connection")
	ErrPeerFrameTruncated          = errors.New("peer frame was truncated")
	ErrPeerReadFailure             = errors.New("peer read failed")
	ErrSourceSessionInterrupted    = errors.New("source session interrupted the connection")
)

type localInterruptState interface {
	LocalInterruptObserved() bool
	LocalInterruptCause() error
	ObserveReadAfterLocalInterrupt()
}

func failRuntime(ctx context.Context, runtime *application.SessionRuntime, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
	defer cancel()
	return failRuntimeWithRecoveryRetry(cleanupCtx, runtime, cause)
}

func failRuntimeCoordinated(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	cause error,
	beforeCleanup func(),
) error {
	return failRuntimeCoordinatedWithin(ctx, coordinator, runtime, cause, beforeCleanup, CleanupTimeout)
}

func failRuntimeCoordinatedWithin(
	ctx context.Context,
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	cause error,
	beforeCleanup func(),
	timeout time.Duration,
) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	err := coordinator.Execute(cleanupCtx, func(FrameWriter) error {
		if beforeCleanup != nil {
			beforeCleanup()
		}
		return failRuntimeWithRecoveryRetry(cleanupCtx, runtime, cause)
	})
	if cause != nil && !errors.Is(err, cause) {
		return errors.Join(cause, err)
	}
	return err
}

func failRuntimeWithRecoveryRetry(ctx context.Context, runtime *application.SessionRuntime, cause error) error {
	firstErr := runtime.Fail(ctx, cause)
	if ctx.Err() != nil {
		return firstErr
	}
	snapshot, snapshotErr := runtime.SnapshotContext(ctx)
	if snapshotErr != nil {
		return errors.Join(firstErr, snapshotErr)
	}
	if snapshot.Control.State != control.StateFault {
		return firstErr
	}
	return errors.Join(firstErr, runtime.Fail(ctx, cause))
}
