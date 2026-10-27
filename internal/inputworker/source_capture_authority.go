package inputworker

import (
	"context"
	"time"
)

// Execution and boundary guards require full fresh proofs, independently of
// the cheaper read-side admission policy for a queued command.
func authorizeSourceExecution(ctx context.Context, service, agent *Channel) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := service.authorizeSourceInput(ctx); err != nil {
		return sourceQueryOwner("service", err)
	}
	return sourceQueryOwner("agent", agent.authorizeSourceInput(ctx))
}

func sourceCaptureAuthorization(ctx context.Context, service, agent *Channel) func() error {
	return func() error {
		// Capture/desktop admission always needs the full proof, under the
		// same owned-query budget as a full authenticated source command.
		return authorizeSourceExecution(ctx, service, agent)
	}
}
