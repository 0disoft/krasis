package peerupdate

import (
	"context"
	"net/http"
	"time"
)

// Diagnostic storage and network waits have their own lane. They cannot hold
// up bundle delivery, app-open requests or clipboard traffic.
func (m *Manager) runDiagnosticControl(ctx context.Context, client *http.Client, address, peer string, authorized func() bool) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			m.mu.Lock()
			at := m.peers[peer].at
			m.mu.Unlock()
			if at.IsZero() || time.Since(at) >= 10*time.Second || !authorized() {
				continue
			}
			m.pollDiagnostic(ctx, client, address, peer, authorized)
			m.pushDiagnosticRequest(ctx, client, address, peer, authorized)
			m.pollDiagnosticRequest(ctx, client, address, peer, authorized)
		}
	}
}
