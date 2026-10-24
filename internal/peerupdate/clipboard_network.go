package peerupdate

import (
	"context"
	"io"
	"net/http"
	"time"
)

func clipboardContentType(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "dib":
		return "application/x-krasis-dib"
	case "files":
		return "application/x-krasis-files+zip"
	default:
		return "text/plain; charset=utf-8"
	}
}

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
		transfer, stop, current := m.clipboardTransferContext(r.Context(), peer, id, value.Format, value.size, true)
		defer stop()
		if transfer.Err() != nil || !current() || !authorized() {
			w.WriteHeader(http.StatusConflict)
			return
		}
		defer interruptClipboardIO(transfer, http.NewResponseController(w).SetWriteDeadline)()
		w.Header().Set("Content-Type", clipboardContentType(value.Format))
		var err error
		if value.Format == "" {
			_, err = io.WriteString(w, value.Text)
		} else {
			reader := &progressReader{Reader: value.body, update: func(n int64) { m.clipboardProgress(peer, id, "SEND", value.Format, n, int64(value.size)) }}
			_, err = io.Copy(w, reader)
		}
		_ = err // A GET write is not receipt or application confirmation; await PUT.
	case http.MethodPost:
		if id != remote.ClipboardID || r.Header.Get("Content-Type") != clipboardContentType(remote.ClipboardFormat) {
			w.WriteHeader(400)
			return
		}
		transfer, stop, current := m.clipboardTransferContext(r.Context(), peer, id, remote.ClipboardFormat, remote.ClipboardSize, false)
		defer stop()
		defer interruptClipboardIO(transfer, http.NewResponseController(w).SetReadDeadline)()
		if err := m.receiveClipboardPayload(peer, id, remote.ClipboardFormat, r.Body, func() bool { return transfer.Err() == nil && current() && authorized() }, true); err != nil {
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

// AfterFunc's stop does not wait for a callback that already started. Join it
// before the handler returns so it cannot change a reused connection's deadline.
func interruptClipboardIO(ctx context.Context, setDeadline func(time.Time) error) func() {
	finished := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(finished)
		_ = setDeadline(time.Now())
	})
	return func() {
		if !stop() {
			<-finished
		}
	}
}

func (m *Manager) pollClipboard(ctx context.Context, c *http.Client, address, peer string, authorized func() bool) {
	remote, ok := m.remoteClipboard(peer)
	local, _, _ := m.ClipboardInfo(peer)
	if !ok || !local || !authorized() {
		return
	}
	_, id, _ := m.ClipboardInfo(peer)
	outgoing := m.clipboardOutgoing(peer, id)
	url := "https://" + address + "/v1/clipboard"
	if value := outgoing; value != nil &&
		(value.Format == "" || remote.Generation >= 69) &&
		(value.Format != "files" || remote.Generation >= 70 &&
			(remote.Generation >= 72 || value.legacyFiles)) {
		m.sendClipboard(ctx, c, url, peer, value, authorized)
	}
	remote, ok = m.remoteClipboard(peer)
	if !ok || remote.ClipboardID == "" || !authorized() || ctx.Err() != nil {
		return
	}
	ctx, stop, current := m.clipboardTransferContext(ctx, peer, remote.ClipboardID, remote.ClipboardFormat, remote.ClipboardSize, false)
	defer stop()
	if !m.clipboardSeen(peer, remote.ClipboardID) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("X-Krasis-ID", remote.ClipboardID)
		resp, err := c.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != clipboardContentType(remote.ClipboardFormat) ||
			!authorized() || m.receiveClipboardPayload(peer, remote.ClipboardID, remote.ClipboardFormat, resp.Body, func() bool { return ctx.Err() == nil && current() && authorized() }, true) != nil {
			return
		}
		resp.Body.Close()
	}
	// Receipt retries still require the current selection, advertised copy and
	// trust; a cached terminal ID must not bypass the transfer's authority fence.
	if ctx.Err() != nil || !current() || !authorized() {
		return
	}
	ack, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	ack.Header.Set("X-Krasis-ID", remote.ClipboardID)
	if resp, err := c.Do(ack); err == nil {
		resp.Body.Close()
	}
}

func clipboardTransferTimeout(format string) time.Duration {
	if format == "files" {
		return 60 * time.Second
	}
	return 12 * time.Second
}

func (m *Manager) sendClipboard(ctx context.Context, c *http.Client, url, peer string, value *clipboardSnapshot, authorized func() bool) {
	ctx, stop, current := m.clipboardTransferContext(ctx, peer, value.ID, value.Format, value.size, true)
	defer stop()
	if !current() || !authorized() {
		return
	}
	body := &progressReader{Reader: value.body, update: func(n int64) {
		m.clipboardProgress(peer, value.ID, "SEND", value.Format, n, int64(value.size))
	}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	req.Header.Set("Content-Type", clipboardContentType(value.Format))
	req.Header.Set("X-Krasis-ID", value.ID)
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 && ctx.Err() == nil && current() && authorized() {
			m.clipboardSent(peer, value.ID)
		}
	}
}
