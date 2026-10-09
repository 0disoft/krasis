package residentagent

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/peer"
)

// Retire before the immutable worker deadline, while release acknowledgements
// can still be obtained. No expired operation is replayed or renewed in place.
func (a privilegedAuthorizer) RunLeaseRetirement(ctx context.Context, returns *peer.ReturnController) error {
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-timer.C:
			if err := a.input.retireDue(ctx, now, returns.Retire); err != nil {
				return err
			}
		}
	}
}

func (input *privilegedLeaseInput) retireDue(ctx context.Context, now time.Time, retire func(context.Context, application.LeaseIdentity) error) error {
	if err := input.mu.LockContext(ctx); err != nil {
		return err
	}
	identity, deadline, active := input.identity, input.leaseExpires, input.client != nil
	input.mu.Unlock()
	if !active || deadline.IsZero() || now.Before(deadline.Add(-10*time.Second)) {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := retire(bounded, identity)
	if errors.Is(err, application.ErrStaleReturnIntent) || errors.Is(err, application.ErrRuntimeNotInbound) {
		return nil // A concurrent boundary return already retired this lease.
	}
	if err != nil {
		return errors.Join(errPrivilegedInput, err)
	}
	if input.event != nil {
		input.event("installed_input_planned_return")
	}
	return nil
}
