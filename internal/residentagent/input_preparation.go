package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/transport"
)

type preparationPolicy interface {
	AuthorizePreparation(context.Context, transport.AuthenticatedPeerContext, time.Time) (bool, error)
}

type preparationCancellation struct {
	cancel   context.CancelFunc
	done     chan struct{}
	deadline time.Time
}

func (input *privilegedLeaseInput) cancelPreparation() {
	if preparation := input.preparationCancel.Load(); preparation != nil {
		preparation.cancel()
	}
}

// Only an already-approved entry may finish existing authenticated preparation.
// No new work is started and the hint's deadline is never renewed. Missing,
// expired or unexpectedly long deadlines retain the old immediate cancellation.
// Shutdown always uses cancelPreparation instead of joining optional work.
func (input *privilegedLeaseInput) finishPreparationForEntry(ctx context.Context) bool {
	p := input.preparationCancel.Load()
	if p == nil {
		return false
	}
	remaining := time.Until(p.deadline)
	if p.done == nil || p.deadline.IsZero() || remaining <= 0 || remaining > agent.InputPreparationTimeout {
		p.cancel()
		return false
	}
	select {
	case <-p.done:
		return true
	case <-ctx.Done():
		p.cancel()
		return false
	}
}

// A fresh trust/permission check permits only preparation. Actual lease
// approval and bootstrap are repeated independently for every handoff.
func (a productAuthorizer) AuthorizePreparation(ctx context.Context, p transport.AuthenticatedPeerContext, now time.Time) (bool, error) {
	if ctx == nil || ctx.Err() != nil {
		return false, errPrivilegedInput
	}
	// Read the current key, role, permission and revocation in one locked
	// snapshot. Two separate reads repeat filesystem/ACL work and can combine
	// a certificate verdict with a policy changed between the two snapshots.
	return a.policy.InboundControlAllowed(p, now)
}

func (a privilegedAuthorizer) PrepareInput(ctx context.Context, p transport.AuthenticatedPeerContext, now time.Time) {
	policy, ok := a.LeaseAuthorizer.(preparationPolicy)
	if !ok || a.input == nil || ctx == nil || ctx.Err() != nil {
		return
	}
	allowed, err := policy.AuthorizePreparation(ctx, p, now)
	if err != nil || !allowed || ctx.Err() != nil {
		return
	}
	input := a.input
	prepared, err := input.prepareIdle(ctx)
	if !prepared {
		return
	}
	// Diagnostic callbacks do not own installation pins or input authority.
	// A slow receipt observer must not retain the lease admission mutex.
	if input.event != nil {
		if err == nil {
			input.event("installed_input_preparation_acknowledged")
		} else {
			input.event("installed_input_preparation_missed")
		}
	}
}

func (input *privilegedLeaseInput) prepareIdle(ctx context.Context) (bool, error) {
	// This also joins preparation before shutdown releases installation pins,
	// and prevents preparation from competing with an approved active lease.
	if err := input.mu.LockContext(ctx); err != nil {
		return false, err
	}
	lockedAt := time.Now()
	defer input.mu.Unlock()
	if input.stopped || input.client != nil || input.entryWaiting.Load() != 0 || input.prepare == nil || (!input.expires.IsZero() && !input.expires.After(time.Now())) || ctx.Err() != nil {
		return false, nil
	}
	input.cursor.mu.Lock()
	presenting := input.cursor.cancel != nil
	input.cursor.mu.Unlock()
	if presenting {
		// The passive owner already uses the serial service endpoint. A hint
		// must not wait behind it while holding the input admission mutex.
		// Actual entry retires/join it before opening the next input lease.
		return false, nil
	}
	input.preparationStarted = lockedAt
	input.preparationFinished = time.Time{}
	run, cancel := context.WithCancel(ctx)
	deadline, _ := run.Deadline()
	preparation := &preparationCancellation{cancel: cancel, done: make(chan struct{}), deadline: deadline}
	input.preparationCancel.Store(preparation)
	defer func() {
		input.preparationCancel.CompareAndSwap(preparation, nil)
		cancel()
		close(preparation.done)
	}()
	// Admission can arrive between the first priority check and publication.
	// Cancel that race too; input.mu still fences native cleanup and pin disposal.
	if input.entryWaiting.Load() != 0 {
		cancel()
	}
	// Runs before the earlier Unlock defer. Only preparation and its receipt
	// own input.mu; the diagnostic callback runs after this helper returns.
	defer func() { input.preparationFinished = time.Now() }()
	return true, input.prepare(run)
}

// Both intervals use local monotonic readings. Only the most recent completed
// preparation is retained under input.mu. Its overlap is a lower bound on
// preparation-related lock contention, not a complete mutex-owner history.
func preparationLockOverlap(waitStarted, acquired, preparationStarted, preparationFinished time.Time) time.Duration {
	if waitStarted.IsZero() || acquired.Before(waitStarted) || preparationStarted.IsZero() || preparationFinished.Before(preparationStarted) {
		return 0
	}
	start, end := waitStarted, acquired
	if preparationStarted.After(start) {
		start = preparationStarted
	}
	if preparationFinished.Before(end) {
		end = preparationFinished
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}
