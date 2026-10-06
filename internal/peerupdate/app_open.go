package peerupdate

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type appOpenRequest struct {
	peer, id, code string
	respondBy      time.Time
	expires        time.Time
}

// Stop advertising a request as soon as its response deadline passes, even
// when the UI is closed and nobody is polling STATUS. An acknowledged launch
// retains the longer window-readiness deadline.
func (r *appOpenRequest) expire(now time.Time) {
	if r.code == "pending" && !now.Before(r.respondBy) {
		r.code = "peer_unresponsive"
	}
	if r.code == "launch_requested" && !now.Before(r.expires) {
		r.code = "window_unconfirmed"
	}
}

type appOpenReceipt struct {
	id, code string
	at       time.Time
}

type appOpenState struct {
	mu       sync.Mutex
	out      *appOpenRequest
	received map[string]appOpenReceipt
}

// QueueAppOpen requests only the receiver's remembered UI, never an executable path.
func (m *Manager) QueueAppOpen(peer string) (string, error) {
	if !validID(peer, 64) {
		return "", ErrDenied
	}
	m.appOpen.mu.Lock()
	defer m.appOpen.mu.Unlock()
	now := time.Now()
	if old := m.appOpen.out; old != nil {
		old.expire(now)
		if old.code == "pending" || old.code == "launch_requested" {
			return "", ErrBusy
		}
	}
	id := randomID()
	m.appOpen.out = &appOpenRequest{peer: peer, id: id, code: "pending", respondBy: now.Add(20 * time.Second), expires: now.Add(time.Minute)}
	return id, nil
}

func (m *Manager) AppOpenStatus(peer, id string) string {
	m.appOpen.mu.Lock()
	defer m.appOpen.mu.Unlock()
	r := m.appOpen.out
	if r == nil || r.peer != peer || r.id != id {
		return "unknown_request"
	}
	r.expire(time.Now())
	return r.code
}

func (m *Manager) appOpenOffer(peer string) string {
	m.appOpen.mu.Lock()
	defer m.appOpen.mu.Unlock()
	r := m.appOpen.out
	if r == nil || r.peer != peer {
		return ""
	}
	r.expire(time.Now())
	if r.code != "pending" && r.code != "launch_requested" {
		return ""
	}
	return r.id
}

func validAppOpenCode(code string) bool {
	switch code {
	case "launch_requested", "window_ready", "launch_failed", "launch_record_missing", "launch_record_invalid", "rejected", "unsupported", "busy":
		return true
	}
	return false
}

func (m *Manager) finishAppOpen(peer, id, code string) {
	if !validAppOpenCode(code) {
		return
	}
	m.appOpen.mu.Lock()
	defer m.appOpen.mu.Unlock()
	if r := m.appOpen.out; r != nil && r.peer == peer && r.id == id {
		r.expire(time.Now())
		if r.code == "pending" || r.code == "launch_requested" {
			r.code = code
		}
	}
}

// Receipt retention and rate limiting bound repeated requests after a lost response.
func (m *Manager) acceptAppOpen(peer, id string, authorized func() bool) string {
	if !validID(peer, 64) || !validID(id, 32) || !authorized() {
		return "rejected"
	}
	if m.OpenUI == nil {
		return "unsupported"
	}
	m.appOpen.mu.Lock()
	defer m.appOpen.mu.Unlock()
	now := time.Now()
	if old, ok := m.appOpen.received[peer]; ok {
		if old.id == id && now.Sub(old.at) < 2*time.Minute {
			if old.code == "launch_requested" && m.UIReady != nil && m.UIReady(old.at) {
				old.code = "window_ready"
				m.appOpen.received[peer] = old
			}
			return old.code
		}
		if now.Sub(old.at) < 3*time.Second {
			return "busy"
		}
	}
	if m.appOpen.received == nil {
		m.appOpen.received = make(map[string]appOpenReceipt)
	}
	for key, receipt := range m.appOpen.received {
		if now.Sub(receipt.at) >= 2*time.Minute {
			delete(m.appOpen.received, key)
		}
	}
	if len(m.appOpen.received) >= 64 {
		return "busy"
	}
	if !authorized() {
		return "rejected"
	}
	code := m.OpenUI(peer)
	if !validAppOpenCode(code) {
		code = "launch_failed"
	}
	m.appOpen.received[peer] = appOpenReceipt{id, code, now}
	return code
}

func (m *Manager) appOpenHTTP(w http.ResponseWriter, r *http.Request, peer string, authorized func() bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.WriteHeader(405)
		return
	}
	if r.ContentLength != 0 {
		w.WriteHeader(400)
		return
	}
	if r.Method == http.MethodGet {
		id := m.appOpenOffer(peer)
		if id == "" {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("X-Krasis-Request-ID", id)
		w.WriteHeader(200)
		return
	}
	id := r.Header.Get("X-Krasis-Request-ID")
	if !validID(id, 32) {
		w.WriteHeader(400)
		return
	}
	if r.Method == http.MethodPost {
		w.Header().Set("X-Krasis-Result", m.acceptAppOpen(peer, id, authorized))
	} else {
		code := r.Header.Get("X-Krasis-Result")
		if !validAppOpenCode(code) {
			w.WriteHeader(400)
			return
		}
		m.finishAppOpen(peer, id, code)
	}
	w.WriteHeader(200)
}

func (m *Manager) pollAppOpen(ctx context.Context, client *http.Client, address, peer string, authorized func() bool) {
	if !authorized() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	url := "https://" + address + "/v1/app-open"
	if id := m.appOpenOffer(peer); id != "" {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		req.Header.Set("X-Krasis-Request-ID", id)
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			if authorized() {
				if resp.StatusCode == 404 {
					m.finishAppOpen(peer, id, "unsupported")
				} else if resp.StatusCode == 200 {
					m.finishAppOpen(peer, id, resp.Header.Get("X-Krasis-Result"))
				}
			}
		}
	}
	if m.OpenUI == nil || !authorized() {
		return
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || !authorized() {
		return
	}
	id := resp.Header.Get("X-Krasis-Request-ID")
	if !validID(id, 32) {
		return
	}
	code := m.acceptAppOpen(peer, id, authorized)
	if !authorized() {
		return
	}
	ack, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	ack.Header.Set("X-Krasis-Request-ID", id)
	ack.Header.Set("X-Krasis-Result", code)
	if result, err := client.Do(ack); err == nil {
		result.Body.Close()
	}
}
