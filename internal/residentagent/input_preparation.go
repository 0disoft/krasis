package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

type preparationPolicy interface {
	AuthorizePreparation(context.Context, transport.AuthenticatedPeerContext, time.Time) (bool, error)
}

// A fresh trust/permission check permits only preparation. Actual lease
// approval and bootstrap are repeated independently for every handoff.
func (a productAuthorizer) AuthorizePreparation(ctx context.Context, p transport.AuthenticatedPeerContext, now time.Time) (bool, error) {
	if ctx == nil || ctx.Err() != nil {
		return false, errPrivilegedInput
	}
	if _, err := a.trust.PeerCertificate(p, now); err != nil {
		return false, err
	}
	document, err := a.trust.List()
	if err != nil {
		return false, err
	}
	for _, record := range document.Peers {
		if record.Fingerprint == p.CertificateSHA256 && record.Role == p.Role {
			return record.AllowInboundControl && record.RevokedAt == nil, nil
		}
	}
	return false, nil
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
	// This also joins preparation before shutdown releases installation pins,
	// and prevents preparation from competing with an approved active lease.
	input.mu.Lock()
	lockedAt := time.Now()
	defer input.mu.Unlock()
	if input.stopped || input.client != nil || input.prepare == nil || !input.expires.After(time.Now()) || ctx.Err() != nil {
		return
	}
	input.preparationStarted = lockedAt
	input.preparationFinished = time.Time{}
	// Runs before the earlier Unlock defer. The interval includes the receipt
	// and event callback while the preparation operation still owns input.mu.
	defer func() { input.preparationFinished = time.Now() }()
	err = input.prepare(ctx)
	if input.event != nil {
		if err == nil {
			input.event("installed_input_preparation_acknowledged")
		} else {
			input.event("installed_input_preparation_missed")
		}
	}
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
