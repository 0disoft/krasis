package peerupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

const MaxDiagnosticBytes = 5 << 20
const diagnosticPollTimeout = 15 * time.Second
const diagnosticReceiptRetention = 2 * time.Minute

// Keep identity and content certainty without retaining another payload copy.
type diagnosticReceipt struct {
	id, requestID string
	digest        [sha256.Size]byte
	expires       time.Time
}

type diagnosticTransfer struct {
	mu              sync.Mutex
	preparing       bool
	receiving       bool
	out             *diagnosticOffer
	request         *diagnosticRequest
	state, received string
	last            map[string]diagnosticReceipt
	handled         map[string]string
	export          *diagnosticExport
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
	return m.PrepareDiagnosticRequest(peer, func(string) (string, error) { return localPath, nil })
}

// Publish only after the local snapshot is pinned to the allocated request ID.
func (m *Manager) PrepareDiagnosticRequest(peer string, save func(string) (string, error)) (string, error) {
	if !validID(peer, 64) || save == nil {
		return "", ErrDenied
	}
	m.diagnostic.mu.Lock()
	if r := m.diagnostic.request; m.diagnostic.preparing || m.diagnostic.receiving || (r != nil && r.state == "PENDING" && time.Now().Before(r.expires)) {
		m.diagnostic.mu.Unlock()
		return "", ErrBusy
	}
	m.diagnostic.preparing = true
	m.diagnostic.mu.Unlock()
	defer func() {
		m.diagnostic.mu.Lock()
		m.diagnostic.preparing = false
		m.diagnostic.mu.Unlock()
	}()
	id := randomID()
	localPath, err := save(id)
	if err != nil {
		return "", err
	}
	if localPath == "" {
		return "", ErrDenied
	}
	m.diagnostic.mu.Lock()
	m.diagnostic.request = &diagnosticRequest{peer: peer, id: id, state: "PENDING", localPath: localPath, expires: time.Now().Add(time.Minute)}
	m.diagnostic.mu.Unlock()
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
	return m.diagnosticRequestHandledLocked(peer, id)
}

func (m *Manager) diagnosticRequestHandledLocked(peer, id string) bool {
	m.expireDiagnosticOfferLocked(time.Now())
	return validID(peer, 64) && validID(id, 32) && m.diagnostic.handled[peer] == id
}

// Caller owns diagnostic.mu. Retire the payload and its unsent request marker
// together, regardless of which observation or queue operation detects expiry.
// Confirmed receipts and markers for a different request must remain intact.
func (m *Manager) expireDiagnosticOfferLocked(now time.Time) {
	out := m.diagnostic.out
	if out == nil || !now.After(out.expires) {
		return
	}
	if out.requestID != "" && m.diagnostic.handled[out.peer] == out.requestID {
		delete(m.diagnostic.handled, out.peer)
	}
	m.diagnostic.out = nil
	m.diagnostic.state = "EXPIRED"
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrDenied
	}
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if job := m.diagnostic.export; job != nil && job.peer == peer && job.id == requestID && job.ctx.Err() != nil {
		return job.ctx.Err()
	}
	m.expireDiagnosticOfferLocked(time.Now())
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
	m.expireDiagnosticOfferLocked(time.Now())
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
	m.expireDiagnosticOfferLocked(time.Now())
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
	if !validID(peer, 64) || !validID(id, 32) || (requestID != "" && !validID(requestID, 32)) || !authorized() || (m.ReceiveDiagnostic == nil && (requestID == "" || m.ReceiveCollection == nil)) {
		return ErrDenied
	}
	// HTTP handlers and outbound polls must share admission before body allocation.
	if !m.beginDiagnosticReceive() {
		return ErrBusy
	}
	defer func() {
		m.diagnostic.mu.Lock()
		m.diagnostic.receiving = false
		m.diagnostic.mu.Unlock()
	}()
	data, err := io.ReadAll(io.LimitReader(r, MaxDiagnosticBytes+1))
	if err != nil || len(data) > MaxDiagnosticBytes {
		return ErrDenied
	}
	if !authorized() {
		return ErrDenied
	}
	digest := sha256.Sum256(data)
	replayed, err := m.validateDiagnosticReceive(peer, id, requestID, digest)
	if err != nil || replayed {
		return err
	}
	var path string
	if requestID != "" && m.ReceiveCollection != nil {
		path, err = m.ReceiveCollection(peer, requestID, data)
	} else {
		path, err = m.ReceiveDiagnostic(peer, data)
	}
	if err != nil {
		return err
	}
	if path == "" {
		return ErrDenied
	}
	if !authorized() {
		return ErrDenied
	}
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	// Storage may outlive the request. Revalidate under the publication lock;
	// a late callback must not revive an expired collection or publish a receipt.
	if replayed, err := m.validateDiagnosticReceiveLocked(peer, id, requestID, digest); err != nil || replayed {
		return err
	}
	if m.diagnostic.last == nil {
		m.diagnostic.last = map[string]diagnosticReceipt{}
	}
	m.diagnostic.last[peer] = diagnosticReceipt{id: id, requestID: requestID, digest: digest, expires: time.Now().Add(diagnosticReceiptRetention)}
	m.diagnostic.received = path
	if requestID != "" {
		m.diagnostic.request.state = "RECEIVED"
		m.diagnostic.request.path = path
	}
	return nil
}

// Own one body read and save across all receive paths. Do not hold the state
// lock over network or filesystem I/O, and do not race local snapshot preparation.
func (m *Manager) beginDiagnosticReceive() bool {
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	if m.diagnostic.receiving || m.diagnostic.preparing {
		return false
	}
	m.diagnostic.receiving = true
	return true
}

// Caller owns the receive slot. Validate after reading, then use the same rules
// under the publication lock again after the potentially slow storage callback.
func (m *Manager) validateDiagnosticReceive(peer, id, requestID string, digest [sha256.Size]byte) (bool, error) {
	m.diagnostic.mu.Lock()
	defer m.diagnostic.mu.Unlock()
	return m.validateDiagnosticReceiveLocked(peer, id, requestID, digest)
}

// Caller owns diagnostic.mu. No body read or storage callback runs here.
func (m *Manager) validateDiagnosticReceiveLocked(peer, id, requestID string, digest [sha256.Size]byte) (bool, error) {
	now := time.Now()
	for key, receipt := range m.diagnostic.last {
		if !now.Before(receipt.expires) {
			delete(m.diagnostic.last, key)
		}
	}
	if receipt, ok := m.diagnostic.last[peer]; ok && receipt.id == id {
		if receipt.requestID == requestID && receipt.digest == digest {
			return true, nil
		}
		return false, ErrDenied
	}
	if requestID != "" {
		request := m.diagnostic.request
		if request == nil || request.peer != peer || request.id != requestID || request.state != "PENDING" || now.After(request.expires) {
			return false, ErrDenied
		}
	}
	if _, known := m.diagnostic.last[peer]; !known && len(m.diagnostic.last) >= 32 {
		return false, ErrBusy
	}
	return false, nil
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
		// Match the sender's budget instead of retaining the single receive slot
		// until the server's bulk-transfer ReadTimeout. Context cancellation alone
		// does not interrupt a blocked body Read; join the deadline callback too.
		ctx, cancel := context.WithTimeout(r.Context(), diagnosticPollTimeout)
		defer cancel()
		defer interruptHTTPIO(ctx, http.NewResponseController(w).SetReadDeadline)()
		live := func() bool { return ctx.Err() == nil && authorized() }
		if err := m.receiveDiagnostic(peer, r.Header.Get("X-Krasis-ID"), r.Header.Get("X-Krasis-Request-ID"), r.Body, live); err != nil {
			if errors.Is(err, ErrBusy) {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
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
		if !validID(id, 32) || !m.hasDiagnosticExporter() || !authorized() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err := m.runDiagnosticExport(r.Context(), peer, id, authorized); err != nil {
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
	if !authorized() || !m.hasDiagnosticExporter() {
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
		_ = m.runDiagnosticExport(ctx, peer, id, authorized) // The worker records a sanitized failure event.
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
			switch resp.StatusCode {
			case http.StatusOK:
				m.diagnosticDone(offer.id, "SENT")
			case http.StatusNotFound:
				m.diagnosticDone(offer.id, "UNSUPPORTED")
			case http.StatusTooManyRequests, http.StatusServiceUnavailable:
				// Handler admission (429) and receive admission (503) are temporary.
				// Retry the same transfer at the next poll until its existing deadline.
			default:
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
