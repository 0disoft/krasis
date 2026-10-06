package peerupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (m *Manager) runAppOpenControl(ctx context.Context, client *http.Client, address, peer string, authorized func() bool) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if !authorized() {
				continue
			}
			data, _ := json.Marshal(m.info(peer))
			attempt, cancel := context.WithTimeout(ctx, 6*time.Second)
			req, _ := http.NewRequestWithContext(attempt, http.MethodPost, "https://"+address+"/v2/status", bytes.NewReader(data))
			req.Header.Set("Content-Type", "application/json")
			ok := m.exchangeStatus(client, req, peer)
			cancel()
			if ok && authorized() {
				m.pollAppOpen(ctx, client, address, peer, authorized)
			}
		}
	}
}
