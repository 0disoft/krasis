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
	defer input.mu.Unlock()
	if input.stopped || input.client != nil || input.prepare == nil || !input.expires.After(time.Now()) || ctx.Err() != nil {
		return
	}
	err = input.prepare(ctx)
	if input.event != nil {
		if err == nil {
			input.event("installed_input_preparation_acknowledged")
		} else {
			input.event("installed_input_preparation_missed")
		}
	}
}
