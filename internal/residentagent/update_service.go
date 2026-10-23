package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

type updateWorkerKey struct {
	peer                     transport.AuthenticatedPeerContext
	alias, address, localKey string
}

type updateWorkerLifetime struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Management workers belong to the resident process, never an input lease.
// Reconcile profile changes and revocations; request authorization also checks
// current trust, so the polling interval is not a revocation grace period.
func (o *Operations) maintainUpdates(ctx context.Context, identities *productidentity.Store, trust *truststore.Store) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	maintainUpdateConnections(ctx, trust.List, func(alias string) (productConnection, error) {
		return prepareProductConnection(identities, trust, alias)
	}, func(run context.Context, alias string, prepared productConnection) {
		o.runUpdates(run, alias, prepared, trust)
	}, ticker.C)
}

func maintainUpdateConnections(ctx context.Context, list func() (truststore.Document, error), prepare func(string) (productConnection, error), start func(context.Context, string, productConnection), ticks <-chan time.Time) {
	workers := map[updateWorkerKey]updateWorkerLifetime{}
	defer func() {
		for _, worker := range workers {
			worker.cancel()
		}
	}()
	for ctx.Err() == nil {
		wanted := map[updateWorkerKey]productConnection{}
		doc, err := list()
		if err == nil {
			for _, p := range doc.Peers {
				if p.RevokedAt != nil || len(wanted) >= 16 {
					continue
				}
				prepared, err := prepare(p.Alias)
				if err != nil {
					continue
				}
				// TLS callbacks pin the local key; authorization also captures
				// the alias. Renewing either requires a new connection owner.
				key := updateWorkerKey{peer: prepared.peer, alias: p.Alias, address: prepared.profile.Address, localKey: prepared.localKey}
				wanted[key] = prepared
			}
		}
		// Withdraw old authorization before admitting replacements. Connection
		// workers retain their own cleanup until the manager joins them.
		for key, worker := range workers {
			if _, keep := wanted[key]; !keep || worker.ctx.Err() != nil {
				worker.cancel()
				delete(workers, key)
			}
		}
		for key, prepared := range wanted {
			if _, running := workers[key]; running {
				continue
			}
			run, cancel := productConnectionContext(ctx, prepared.deadline)
			if run.Err() != nil {
				cancel()
				continue
			}
			workers[key] = updateWorkerLifetime{ctx: run, cancel: cancel}
			start(run, key.alias, prepared)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}
