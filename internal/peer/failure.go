package peer

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

const CleanupTimeout = 2 * time.Second

func failRuntime(ctx context.Context, runtime *application.SessionRuntime, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
	defer cancel()
	return runtime.Fail(cleanupCtx, cause)
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
		return runtime.Fail(cleanupCtx, cause)
	})
	if cause != nil && !errors.Is(err, cause) {
		return errors.Join(cause, err)
	}
	return err
}
