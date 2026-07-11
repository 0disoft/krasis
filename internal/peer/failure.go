package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

const CleanupTimeout = 2 * time.Second

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
	if runtime.Snapshot().Control.State != control.StateFault || ctx.Err() != nil {
		return firstErr
	}
	return errors.Join(firstErr, runtime.Fail(ctx, cause))
}
