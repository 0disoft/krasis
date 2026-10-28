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
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrAuthority, err)
	}
	// Independent proofs retain separate pinned handles. Join both bounded
	// callers before starting a worker; neither fresh proof may be skipped.
	serviceResult := make(chan error, 1)
	go func() { serviceResult <- service.authorizeInput(ctx, false) }()
	agentErr := agent.authorizeInput(ctx, false)
	serviceErr := <-serviceResult
	if agentErr != nil || serviceErr != nil || ctx.Err() != nil {
		return errors.Join(ErrAuthority, agentErr, serviceErr, ctx.Err())
	}
	return nil
}
