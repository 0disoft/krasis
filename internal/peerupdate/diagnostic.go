package peerupdate

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

const MaxDiagnosticBytes = 1 << 20

type diagnosticTransfer struct {
	mu              sync.Mutex
	out             *diagnosticOffer
	state, received string
	last            map[string]string
}
type diagnosticOffer struct {
	peer, id string
	data     []byte
	expires  time.Time
}

func (m *Manager) QueueDiagnostic(peer string, data []byte) error {
	if !validID(peer, 64) || len(data) == 0 || len(data) > MaxDiagnosticBytes {
		return ErrDenied
	}
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if m.diagnostic.out != nil && time.Now().Before(m.diagnostic.out.expires) {
		return ErrBusy
	}
	m.diagnostic.out = &diagnosticOffer{peer: peer, id: randomID(), data: append([]byte(nil), data...), expires: time.Now().Add(time.Minute)}
	m.diagnostic.state = "QUEUED"
	return nil
}
func (m *Manager) diagnosticOffer(peer string) *diagnosticOffer {
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if m.diagnostic.out != nil && time.Now().After(m.diagnostic.out.expires) {
		m.diagnostic.out = nil
		m.diagnostic.state = "EXPIRED"
	}
	if m.diagnostic.out == nil || m.diagnostic.out.peer != peer {
		return nil
	}
	copy := *m.diagnostic.out
	return &copy
}
func (m *Manager) diagnosticDone(id, state string) {
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if m.diagnostic.out != nil && m.diagnostic.out.id == id {
		m.diagnostic.out = nil
		m.diagnostic.state = state
	}
}
func (m *Manager) receiveDiagnostic(peer, id string, r io.Reader, authorized func() bool) error {
	if !validID(id, 32) || !authorized() || m.ReceiveDiagnostic == nil {
		return ErrDenied
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxDiagnosticBytes+1))
	if err != nil || len(data) > MaxDiagnosticBytes {
		return ErrDenied
	}
	if !authorized() {
		return ErrDenied
	}
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if m.diagnostic.last[peer] == "" && len(m.diagnostic.last) >= 32 {
		return ErrBusy
	}
	if m.diagnostic.last[peer] == id {
		return nil
	}
	path, err := m.ReceiveDiagnostic(peer, data)
	if err != nil {
		return err
	}
	if m.diagnostic.last == nil {
		m.diagnostic.last = map[string]string{}
	}
	m.diagnostic.last[peer] = id
	m.diagnostic.received = path
	return nil
}
func (m *Manager) diagnosticHTTP(w http.ResponseWriter, r *http.Request, peer string, authorized func() bool) {
	switch r.Method {
	case http.MethodGet:
		offer := m.diagnosticOffer(peer)
		if offer == nil {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("X-Krasis-ID", offer.id)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(offer.data)
	case http.MethodPost:
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(400)
			return
		}
		if err := m.receiveDiagnostic(peer, r.Header.Get("X-Krasis-ID"), r.Body, authorized); err != nil {
			w.WriteHeader(422)
			return
		}
		w.WriteHeader(200)
	case http.MethodPut:
		if r.ContentLength != 0 {
			w.WriteHeader(400)
			return
		}
		offer := m.diagnosticOffer(peer)
		if offer != nil && offer.id == r.Header.Get("X-Krasis-ID") {
			m.diagnosticDone(offer.id, "SENT")
		}
		w.WriteHeader(200)
	default:
		w.WriteHeader(405)
	}
}
func (m *Manager) pollDiagnostic(ctx context.Context, c *http.Client, address, peer string, authorized func() bool) {
	if !authorized() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url := "https://" + address + "/v1/diagnostic-transfer"
	if offer := m.diagnosticOffer(peer); offer != nil {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(offer.data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Krasis-ID", offer.id)
		resp, err := c.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				m.diagnosticDone(offer.id, "SENT")
			} else if resp.StatusCode == 404 {
				m.diagnosticDone(offer.id, "UNSUPPORTED")
			} else {
				m.diagnosticDone(offer.id, "REJECTED")
			}
		}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := c.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	id := resp.Header.Get("X-Krasis-ID")
	if m.receiveDiagnostic(peer, id, resp.Body, authorized) != nil {
		return
	}
	resp.Body.Close()
	ack, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	ack.Header.Set("X-Krasis-ID", id)
	if result, err := c.Do(ack); err == nil {
		result.Body.Close()
	}
}
