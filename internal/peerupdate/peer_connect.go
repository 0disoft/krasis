package peerupdate

import (
	"context"
	"net/http"
	"sync"
	"time"
)

const peerConnectPath = "/v1/peer-connect"
const peerConnectLifetime = time.Minute

type peerConnectRequest struct {
	peer, id, code string
	expires        time.Time
}
type peerConnectReceipt struct {
	id, code string
	at       time.Time
}
type peerConnectState struct {
	mu       sync.Mutex
	out      *peerConnectRequest
	received map[string]peerConnectReceipt
	active   bool
}

func validPeerConnectCode(code string) bool {
	switch code {
	case "accepted", "connected", "busy", "rejected", "unsupported", "connection_unconfirmed":
		return true
	}
	return false
}

func (m *Manager) QueuePeerConnect(peer string) (string, error) {
	if !validID(peer, 64) {
		return "", ErrDenied
	}
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return "", ErrDenied
	}
	m.peerConnect.mu.Lock()
	defer m.peerConnect.mu.Unlock()
	now := time.Now()
	if r := m.peerConnect.out; r != nil && now.Before(r.expires) && (r.code == "pending" || r.code == "accepted") {
		return "", ErrBusy
	}
	id := randomID()
	m.peerConnect.out = &peerConnectRequest{peer: peer, id: id, code: "pending", expires: now.Add(peerConnectLifetime)}
	return id, nil
}

func (m *Manager) PeerConnectStatus(peer, id string) string {
	m.peerConnect.mu.Lock()
	defer m.peerConnect.mu.Unlock()
	r := m.peerConnect.out
	if r == nil || r.peer != peer || r.id != id {
		return "unknown_request"
	}
	if !time.Now().Before(r.expires) && (r.code == "pending" || r.code == "accepted") {
		r.code = "connection_unconfirmed"
	}
	return r.code
}

func (m *Manager) peerConnectOffer(peer string) string {
	m.peerConnect.mu.Lock()
	defer m.peerConnect.mu.Unlock()
	r := m.peerConnect.out
	if r != nil && r.peer == peer && time.Now().Before(r.expires) && (r.code == "pending" || r.code == "accepted") {
		return r.id
	}
	return ""
}

func (m *Manager) finishPeerConnect(peer, id, code string) {
	if !validPeerConnectCode(code) {
		return
	}
	m.peerConnect.mu.Lock()
	defer m.peerConnect.mu.Unlock()
	if r := m.peerConnect.out; r != nil && r.peer == peer && r.id == id && time.Now().Before(r.expires) && (r.code == "pending" || r.code == "accepted") {
		r.code = code
	}
}

// One bounded, cancelable job per manager; exact receipts prevent a lost HTTP
// response from starting the connection twice. No address, program or input is accepted.
func (m *Manager) acceptPeerConnect(peer, id string, authorized func() bool) string {
	if !validID(peer, 64) || !validID(id, 32) || !authorized() {
		return "rejected"
	}
	if m.ConnectPeer == nil {
		return "unsupported"
	}
	s := &m.peerConnect
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if old, ok := s.received[peer]; ok {
		if old.id == id && now.Sub(old.at) < 2*time.Minute {
			return old.code
		}
		if now.Sub(old.at) < 3*time.Second {
			return "busy"
		}
	}
	if s.active {
		return "busy"
	}
	if s.received == nil {
		s.received = make(map[string]peerConnectReceipt)
	}
	for key, r := range s.received {
		if now.Sub(r.at) >= 2*time.Minute {
			delete(s.received, key)
		}
	}
	if len(s.received) >= 64 || !authorized() {
		return "rejected"
	}
	ctx, cancel := context.WithTimeout(context.Background(), peerConnectLifetime)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cancel()
		return "rejected"
	}
	m.nextWorker++
	token := m.nextWorker
	if m.cancels == nil {
		m.cancels = make(map[uint64]context.CancelFunc)
	}
	m.cancels[token] = cancel
	m.workers.Add(1)
	m.mu.Unlock()
	s.active = true
	s.received[peer] = peerConnectReceipt{id: id, code: "accepted", at: now}
	go func() {
		defer m.workers.Done()
		defer cancel()
		defer func() { m.mu.Lock(); delete(m.cancels, token); m.mu.Unlock() }()
		code := "rejected"
		if ctx.Err() == nil && authorized() {
			code = m.ConnectPeer(ctx, peer)
		}
		if !validPeerConnectCode(code) || code == "accepted" {
			code = "connection_unconfirmed"
		}
		if ctx.Err() != nil {
			code = "connection_unconfirmed"
		}
		if !authorized() {
			code = "rejected"
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.active = false
		if r := s.received[peer]; r.id == id {
			r.code = code
			s.received[peer] = r
		}
	}()
	return "accepted"
}

func (m *Manager) peerConnectHTTP(w http.ResponseWriter, r *http.Request, peer string, authorized func() bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.WriteHeader(405)
		return
	}
	if r.ContentLength != 0 {
		w.WriteHeader(400)
		return
	}
	if r.Method == http.MethodGet {
		id := m.peerConnectOffer(peer)
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
		w.Header().Set("X-Krasis-Result", m.acceptPeerConnect(peer, id, authorized))
	} else {
		code := r.Header.Get("X-Krasis-Result")
		if !validPeerConnectCode(code) {
			w.WriteHeader(400)
			return
		}
		m.finishPeerConnect(peer, id, code)
	}
	w.WriteHeader(200)
}

func (m *Manager) pollPeerConnect(ctx context.Context, client *http.Client, address, peer string, authorized func() bool) {
	if !authorized() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	url := "https://" + address + peerConnectPath
	if id := m.peerConnectOffer(peer); id != "" {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		req.Header.Set("X-Krasis-Request-ID", id)
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			if authorized() {
				if resp.StatusCode == 404 {
					m.finishPeerConnect(peer, id, "unsupported")
				} else if resp.StatusCode == 200 {
					m.finishPeerConnect(peer, id, resp.Header.Get("X-Krasis-Result"))
				}
			}
		}
	}
	if m.ConnectPeer == nil || !authorized() {
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
	code := m.acceptPeerConnect(peer, id, authorized)
	if !authorized() {
		return
	}
	ack, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	ack.Header.Set("X-Krasis-Request-ID", id)
	ack.Header.Set("X-Krasis-Result", code)
	if r, err := client.Do(ack); err == nil {
		r.Body.Close()
	}
}
