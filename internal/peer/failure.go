package peer

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

const CleanupTimeout = 2 * time.Second

func failRuntime(ctx context.Context, runtime *application.SessionRuntime, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), CleanupTimeout)
	defer cancel()
	return runtime.Fail(cleanupCtx, cause)
}
