package peerupdate

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

const MaxDiagnosticBytes = 5 << 20
const diagnosticPollTimeout = 15 * time.Second

type diagnosticTransfer struct {
	mu              sync.Mutex
	out             *diagnosticOffer
	request         *diagnosticRequest
	state, received string
	last            map[string]string
	handled         map[string]string
}
type diagnosticOffer struct {
	peer, id, requestID string
	data                []byte
	expires             time.Time
}
type diagnosticRequest struct {
	peer, id, state, path, localPath string
	expires                          time.Time
}

// QueueDiagnosticRequest offers a one-minute, peer-bound request. Only a
// matching response is allowed to complete it.
func (m *Manager) QueueDiagnosticRequest(peer, localPath string) (string, error) {
	if !validID(peer, 64) {
		return "", ErrDenied
	}
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if r := m.diagnostic.request; r != nil && r.state == "PENDING" && time.Now().Before(r.expires) {
		return "", ErrBusy
	}
	id := randomID()
	m.diagnostic.request = &diagnosticRequest{peer: peer, id: id, state: "PENDING", localPath: localPath, expires: time.Now().Add(time.Minute)}
	return id, nil
}

func (m *Manager) DiagnosticRequestStatus(peer, id string) (string, string, string) {
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	r := m.diagnostic.request
	if r == nil || r.peer != peer || r.id != id {
		return "UNKNOWN", "", ""
	}
	if r.state == "PENDING" && time.Now().After(r.expires) {
		r.state = "EXPIRED"
	}
	return r.state, r.localPath, r.path
}

func (m *Manager) DiagnosticRequestHandled(peer, id string) bool {
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if m.diagnostic.out != nil && time.Now().After(m.diagnostic.out.expires) {
		if m.diagnostic.out.requestID != "" && m.diagnostic.handled[m.diagnostic.out.peer] == m.diagnostic.out.requestID {
			delete(m.diagnostic.handled, m.diagnostic.out.peer)
		}
		m.diagnostic.out = nil
		m.diagnostic.state = "EXPIRED"
	}
	return validID(peer, 64) && validID(id, 32) && m.diagnostic.handled[peer] == id
}

func (m *Manager) diagnosticRequestOffer(peer string) string {
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	r := m.diagnostic.request
	if r == nil || r.peer != peer || r.state != "PENDING" {
		return ""
	}
	if time.Now().After(r.expires) {
		r.state = "EXPIRED"
		return ""
	}
	return r.id
}

// A repeated poll or lost response cannot trigger a second export for one ID.
func (m *Manager) QueueDiagnosticForRequest(peer, requestID string, data []byte) error {
	if !validID(requestID, 32) || !validID(peer, 64) || len(data) == 0 || len(data) > MaxDiagnosticBytes {
		return ErrDenied
	}
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if m.diagnostic.out != nil && time.Now().After(m.diagnostic.out.expires) {
		if m.diagnostic.out.requestID != "" && m.diagnostic.handled[m.diagnostic.out.peer] == m.diagnostic.out.requestID {
			delete(m.diagnostic.handled, m.diagnostic.out.peer)
		}
		m.diagnostic.out = nil
		m.diagnostic.state = "EXPIRED"
	}
	if m.diagnostic.handled[peer] == requestID {
		return nil
	}
	if m.diagnostic.out != nil && time.Now().Before(m.diagnostic.out.expires) {
		return ErrBusy
	}
	if m.diagnostic.handled[peer] == "" && len(m.diagnostic.handled) >= 32 {
		return ErrBusy
	}
	m.diagnostic.out = &diagnosticOffer{peer: peer, id: randomID(), requestID: requestID, data: append([]byte(nil), data...), expires: time.Now().Add(time.Minute)}
	m.diagnostic.state = "QUEUED"
	if m.diagnostic.handled == nil {
		m.diagnostic.handled = map[string]string{}
	}
	m.diagnostic.handled[peer] = requestID
	return nil
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
		if m.diagnostic.out.requestID != "" && m.diagnostic.handled[m.diagnostic.out.peer] == m.diagnostic.out.requestID {
			delete(m.diagnostic.handled, m.diagnostic.out.peer)
		}
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
		if state != "SENT" && m.diagnostic.out.requestID != "" && m.diagnostic.handled[m.diagnostic.out.peer] == m.diagnostic.out.requestID {
			delete(m.diagnostic.handled, m.diagnostic.out.peer)
		}
		m.diagnostic.out = nil
		m.diagnostic.state = state
	}
}
func (m *Manager) receiveDiagnostic(peer, id, requestID string, r io.Reader, authorized func() bool) error {
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
	if requestID != "" {
		request := m.diagnostic.request
		if !validID(requestID, 32) || request == nil || request.peer != peer || request.id != requestID || request.state != "PENDING" || time.Now().After(request.expires) {
			return ErrDenied
		}
	}
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
	if requestID != "" {
		m.diagnostic.request.state = "RECEIVED"
		m.diagnostic.request.path = path
	}
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
		if offer.requestID != "" {
			w.Header().Set("X-Krasis-Request-ID", offer.requestID)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(offer.data)
	case http.MethodPost:
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(400)
			return
		}
		if err := m.receiveDiagnostic(peer, r.Header.Get("X-Krasis-ID"), r.Header.Get("X-Krasis-Request-ID"), r.Body, authorized); err != nil {
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
func (m *Manager) diagnosticRequestHTTP(w http.ResponseWriter, r *http.Request, peer string, authorized func() bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.ContentLength != 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodPost {
		id := r.Header.Get("X-Krasis-Request-ID")
		if !validID(id, 32) || m.ExportDiagnostic == nil || !authorized() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err := m.ExportDiagnostic(peer, id); err != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	id := m.diagnosticRequestOffer(peer)
	if id == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("X-Krasis-Request-ID", id)
	w.WriteHeader(http.StatusOK)
}

func (m *Manager) pushDiagnosticRequest(ctx context.Context, c *http.Client, address, peer string, authorized func() bool) {
	if !authorized() {
		return
	}
	id := m.diagnosticRequestOffer(peer)
	if id == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, diagnosticPollTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+address+"/v1/diagnostic-request", nil)
	req.Header.Set("X-Krasis-Request-ID", id)
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
	}
}

func (m *Manager) pollDiagnosticRequest(ctx context.Context, c *http.Client, address, peer string, authorized func() bool) {
	if !authorized() || m.ExportDiagnostic == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, diagnosticPollTimeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+address+"/v1/diagnostic-request", nil)
	resp, err := c.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !authorized() {
		return
	}
	id := resp.Header.Get("X-Krasis-Request-ID")
	if validID(id, 32) {
		_ = m.ExportDiagnostic(peer, id)
	}
}
func (m *Manager) pollDiagnostic(ctx context.Context, c *http.Client, address, peer string, authorized func() bool) {
	if !authorized() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, diagnosticPollTimeout)
	defer cancel()
	url := "https://" + address + "/v1/diagnostic-transfer"
	if offer := m.diagnosticOffer(peer); offer != nil {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(offer.data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Krasis-ID", offer.id)
		if offer.requestID != "" {
			req.Header.Set("X-Krasis-Request-ID", offer.requestID)
		}
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
	if m.receiveDiagnostic(peer, id, resp.Header.Get("X-Krasis-Request-ID"), resp.Body, authorized) != nil {
		return
	}
	resp.Body.Close()
	ack, _ := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	ack.Header.Set("X-Krasis-ID", id)
	if result, err := c.Do(ack); err == nil {
		result.Body.Close()
	}
}
