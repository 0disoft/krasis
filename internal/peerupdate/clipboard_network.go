package peerupdate

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (m *Manager) remoteClipboard(peer string) (PeerInfo, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.peers[peer]
	return p.info, ok && time.Since(p.at) < 10*time.Second && p.info.Clipboard
}
func (m *Manager) clipboardHTTP(w http.ResponseWriter, r *http.Request, peer string, authorized func() bool) {
	remote, ok := m.remoteClipboard(peer)
	local, _, _ := m.ClipboardInfo(peer)
	if !ok || !local || !authorized() {
		w.WriteHeader(403)
		return
	}
	id := r.Header.Get("X-Krasis-ID")
	switch r.Method {
	case http.MethodGet:
		value := m.clipboardOutgoing(peer, id)
		if value == nil {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(value.Text))
	case http.MethodPost:
		if id != remote.ClipboardID || r.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
			w.WriteHeader(400)
			return
		}
		if err := m.clipboardReceive(peer, id, r.Body, authorized); err != nil {
			w.WriteHeader(409)
			return
		}
		w.WriteHeader(200)
	case http.MethodPut:
		if r.ContentLength != 0 {
			w.WriteHeader(400)
			return
		}
		m.clipboardSent(peer, id)
		w.WriteHeader(200)
	default:
		w.WriteHeader(405)
	}
}
func (m *Manager) pollClipboard(ctx context.Context, c *http.Client, address, peer string, authorized func() bool) {
	remote, ok := m.remoteClipboard(peer)
	local, _, _ := m.ClipboardInfo(peer)
	if !ok || !local || !authorized() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	url := "https://" + address + "/v1/clipboard"
	_, id, _ := m.ClipboardInfo(peer)
	if value := m.clipboardOutgoing(peer, id); value != nil {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(value.Text))
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		req.Header.Set("X-Krasis-ID", id)
		if resp, err := c.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				m.clipboardSent(peer, id)
			}
		}
	}
	if remote.ClipboardID == "" || !authorized() {
		return
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("X-Krasis-ID", remote.ClipboardID)
	resp, err := c.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !authorized() || m.clipboardReceive(peer, remote.ClipboardID, resp.Body, authorized) != nil {
		return
	}
	resp.Body.Close()
	ack, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	ack.Header.Set("X-Krasis-ID", remote.ClipboardID)
	if resp, err := c.Do(ack); err == nil {
		resp.Body.Close()
	}
}
