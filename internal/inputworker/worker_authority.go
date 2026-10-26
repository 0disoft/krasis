package inputworker

import (
	"context"
	"errors"
	"time"
)

// Input startup and passive presentation need both fresh proofs. Their shared
// budget covers native admission and both queries; a timed-out query retains
// its exact pipe/proof handles instead of blocking the journal owner forever.
func authorizeWorkerOwners(ctx context.Context, agent, service *Channel) error {
	if ctx == nil || agent == nil || service == nil || agent.authority == nil || service.authority == nil ||
		agent.authority.kind != authorityAgent || service.authority.kind != authorityService {
		return ErrAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := agent.authorizeInput(ctx, false); err != nil {
		return errors.Join(ErrAuthority, err, ctx.Err())
	}
	if err := service.authorizeInput(ctx, false); err != nil {
		return errors.Join(ErrAuthority, err, ctx.Err())
	}
	return nil
}
