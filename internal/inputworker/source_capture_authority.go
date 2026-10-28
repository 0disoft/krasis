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
	// Independent, read-only proofs retain their per-channel native owners.
	// Join both bounded callers before returning; neither success is cached.
	serviceResult := make(chan error, 1)
	go func() { serviceResult <- service.authorizeSourceInput(ctx) }()
	agentErr := agent.authorizeSourceInput(ctx)
	if err := <-serviceResult; err != nil {
		return sourceQueryOwner("service", err)
	}
	return sourceQueryOwner("agent", agentErr)
}

func sourceCaptureAuthorization(ctx context.Context, service, agent *Channel) func() error {
	return func() error {
		// Capture/desktop admission always needs the full proof, under the
		// same owned-query budget as a full authenticated source command.
		return authorizeSourceExecution(ctx, service, agent)
	}
}
