package inputworker

import (
	"context"
	"time"
)

// Read admission checks both authenticated owners; a heartbeat never grants
// input authority. The hook's desktop-affine authorization remains separate.
func authorizeSourceRead(ctx context.Context, service, agent *Channel, op byte) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	full := func() error {
		if err := service.authorizeSourceInput(ctx); err != nil {
			return err
		}
		return agent.authorizeSourceInput(ctx)
	}
	present := func() error {
		if err := service.authorizeSourcePresence(ctx); err != nil {
			return err
		}
		return agent.authorizeSourcePresence(ctx)
	}
	return sourceCommandReadAuthorization(op, full, present)
}
