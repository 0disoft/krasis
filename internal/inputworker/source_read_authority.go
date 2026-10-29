package inputworker

import (
	"context"
	"time"
)

// Read admission checks both authenticated owners; a heartbeat never grants
// input authority. Actual desktop work remains on its owning Windows thread.
func authorizeSourceRead(ctx context.Context, service, agent *Channel, op byte) error {
	return authorizeSourceReadProgress(ctx, service, agent, op, nil)
}

func authorizeSourceWatchdog(ctx context.Context, service, agent *Channel) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	for _, owner := range []struct {
		name    string
		channel *Channel
	}{{"service", service}, {"agent", agent}} {
		if err := owner.channel.authorizeSourceWatchdogPresence(ctx); err != nil {
			return sourceQueryOwner(owner.name, err)
		}
	}
	return nil
}

func authorizeSourceReadProgress(ctx context.Context, service, agent *Channel, op byte, progress *sourceHeartbeatProgress) error {
	if progress != nil && op == sourceHeartbeat {
		progress.presence, progress.gateBucket = sourcePresenceTiming{}, 0
		ctx = progress.presence.context(ctx)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	full := func() error {
		return authorizeSourceExecution(ctx, service, agent)
	}
	present := func() error {
		check := func(owner *Channel, stage byte) error {
			if progress != nil && op == sourceHeartbeat {
				defer progress.measure(stage)()
			}
			return owner.authorizeSourcePresence(ctx)
		}
		if err := check(service, heartbeatServicePresence); err != nil {
			return sourceQueryOwner("service", err)
		}
		return sourceQueryOwner("agent", check(agent, heartbeatAgentPresence))
	}
	return sourceCommandReadAuthorization(op, full, present)
}
