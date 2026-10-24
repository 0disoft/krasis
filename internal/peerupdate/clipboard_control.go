package peerupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

const clipboardStatusPath = "/v3/clipboard-status"
const clipboardNotifyGeneration = 207

// A dedicated management connection avoids queueing small copies behind bundle
// or diagnostic transfers. Supported peers wait for change rather than poll.
func (m *Manager) runClipboardControl(ctx context.Context, client *http.Client, address, peer string, authorized func() bool) {
	var remoteRevision string
	for ctx.Err() == nil {
		revision, changed := m.ClipboardChanges()
		local, _, _ := m.ClipboardInfo(peer)
		if !local || !authorized() {
			wait, cancel := context.WithTimeout(ctx, time.Second)
			m.WaitClipboardChange(wait, revision)
			cancel()
			continue
		}
		data, _ := json.Marshal(m.info(peer))
		path := clipboardStatusPath
		m.mu.Lock()
		generation := m.peers[peer].info.Generation
		m.mu.Unlock()
		if generation > 0 && generation < clipboardNotifyGeneration {
			path = "/v2/status"
		}
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			select {
			case <-changed:
				cancel()
			case <-attempt.Done():
			}
		}()
		request, _ := http.NewRequestWithContext(attempt, http.MethodPost, "https://"+address+path, bytes.NewReader(data))
		request.Header.Set("Content-Type", "application/json")
		if path == clipboardStatusPath && remoteRevision != "" {
			request.Header.Set("X-Krasis-Clipboard-Revision", remoteRevision)
		}
		response, err := client.Do(request)
		ok := false
		if err == nil {
			if response.StatusCode == http.StatusOK {
				info, readErr := readPeerInfo(response.Body)
				if readErr == nil {
					// A successful response without a usable revision cannot sustain
					// long polling. Discard an older token and use bounded polling.
					remoteRevision = ""
					if version := response.Header.Get("X-Krasis-Clipboard-Revision"); version != "" {
						if _, parseErr := strconv.ParseUint(version, 10, 64); parseErr == nil {
							remoteRevision = version
						}
					}
					// Finish reading before publishing the response's change signal.
					cancel()
					<-stopped
					m.observePeer(peer, info)
					ok = true
				}
			}
			response.Body.Close()
		}
		cancel()
		if !ok {
			<-stopped
		}
		if ok && authorized() {
			m.pollClipboard(ctx, client, address, peer, authorized)
		}
		// Legacy peers, failed requests and responses without a usable revision
		// use bounded backoff. A newly queued local copy interrupts the wait.
		if path != clipboardStatusPath || !ok || remoteRevision == "" {
			current, _ := m.ClipboardChanges()
			if current != revision && ctx.Err() == nil {
				continue
			}
			wait, done := context.WithTimeout(ctx, 200*time.Millisecond)
			m.WaitClipboardChange(wait, current)
			done()
		}
	}
}

func (m *Manager) clipboardStatusHTTP(w http.ResponseWriter, r *http.Request, peer string, authorized func() bool) {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	info, err := readPeerInfo(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var after uint64
	version := r.Header.Get("X-Krasis-Clipboard-Revision")
	if version != "" {
		after, err = strconv.ParseUint(version, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	m.observePeer(peer, info)
	if version != "" {
		wait, cancel := context.WithTimeout(r.Context(), time.Second)
		m.WaitClipboardChange(wait, after)
		cancel()
	}
	if r.Context().Err() != nil {
		return
	}
	if !authorized() {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	revision, _ := m.ClipboardChanges()
	w.Header().Set("X-Krasis-Clipboard-Revision", strconv.FormatUint(revision, 10))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m.info(peer))
}
