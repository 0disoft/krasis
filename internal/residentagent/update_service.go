package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/truststore"
	"time"
)

// Management workers belong to the resident process, never an input lease.
// Reconcile profile changes and revocations; request authorization also checks
// current trust, so the polling interval is not a revocation grace period.
func (o *Operations) maintainUpdates(ctx context.Context, identities *productidentity.Store, trust *truststore.Store) {
	workers := map[string]context.CancelFunc{}
	defer func() {
		for _, cancel := range workers {
			cancel()
		}
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		wanted := map[string]bool{}
		doc, err := trust.List()
		if err == nil {
			for _, p := range doc.Peers {
				if p.RevokedAt != nil || len(wanted) >= 16 {
					continue
				}
				prepared, err := prepareProductConnection(identities, trust, p.Alias)
				if err != nil {
					continue
				}
				key := p.Fingerprint + "|" + prepared.profile.Address + "|" + string(prepared.peer.Role)
				wanted[key] = true
				if workers[key] != nil {
					continue
				}
				run, cancel := productConnectionContext(ctx, prepared.deadline)
				workers[key] = cancel
				o.runUpdates(run, p.Alias, prepared, trust)
			}
		}
		for key, cancel := range workers {
			if !wanted[key] {
				cancel()
				delete(workers, key)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
