package inputworker

import (
	"context"
	"time"
)

// Read admission checks both authenticated owners; a heartbeat never grants
// input authority. Actual desktop work remains on its owning Windows thread.
func authorizeSourceRead(ctx context.Context, service, agent *Channel, op byte) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	full := func() error {
		if err := service.authorizeSourceInput(ctx); err != nil {
			return sourceQueryOwner("service", err)
		}
		return sourceQueryOwner("agent", agent.authorizeSourceInput(ctx))
	}
	present := func() error {
		if err := service.authorizeSourcePresence(ctx); err != nil {
			return sourceQueryOwner("service", err)
		}
		return sourceQueryOwner("agent", agent.authorizeSourcePresence(ctx))
	}
	return sourceCommandReadAuthorization(op, full, present)
}
