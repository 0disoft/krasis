package inputworker

import (
	"context"
	"errors"
	"time"
)

// Both fresh proofs share one budget and retain their native handles through
// query retirement. Neither a lease cancellation nor a wedged SCM/WTS call may
// keep the passive window's OS thread inside an unbounded authorization call.
func authorizeCursorPresentation(ctx context.Context, agent, service *Channel) error {
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

// Query admission ends before the final worker report. A pending proof cannot
// certify retirement; its channel retains exact handles until actual return.
// The service channel owns only a reference to the independent report pipe.
func retireCursorPresentation(result error, agent, service *Channel) error {
	// The window/hooks are already gone. Give canceled read-only queries the
	// existing independent cleanup budget to join, instead of demanding service
	// repair merely because cancellation won a race with a healthy native call.
	ctx, cancel := context.WithTimeout(context.Background(), RecoveryTimeout)
	defer cancel()
	for _, owner := range []*Channel{agent, service} {
		if owner == nil {
			continue
		}
		owner.queryMu.Lock()
		owner.closeQueryAdmissionLocked()
		query := owner.query // Presentation admits only full proof queries.
		owner.queryMu.Unlock()
		if query != nil {
			select {
			case <-query.done:
			case <-ctx.Done():
			}
		}
	}
	return serviceRecoveryFailure(result, errors.Join(agent.Close(), service.Close()))
}
