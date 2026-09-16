package peerupdate

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

// Serve uses the existing paired TLS roles on the adjacent TCP port. Both roles
// can send: a listener offers a download, a connector uploads. No new messages
// enter the latency-sensitive input connection, including when peers are old.
func (m *Manager) Run(ctx context.Context, address, peer string, listener bool, config *tls.Config, authorized func() bool) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n >= 65535 {
		return
	}
	address = net.JoinHostPort(host, strconv.Itoa(n+1))
	cfg := config.Clone()
	cfg.NextProtos = []string{"http/1.1"}
	cfg.SessionTicketsDisabled = true
	if listener {
		ln, err := tls.Listen("tcp4", address, cfg)
		if err != nil {
			return
		}
		s := &http.Server{Handler: m.handler(peer, authorized), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 90 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 4096}
		stop := context.AfterFunc(ctx, func() { _ = s.Close() })
		defer stop()
		defer ln.Close()
		_ = s.Serve(ln)
		return
	}
	t := &http.Transport{TLSClientConfig: cfg, DisableKeepAlives: true, DisableCompression: true, MaxConnsPerHost: 1, ResponseHeaderTimeout: 5 * time.Second, TLSHandshakeTimeout: 5 * time.Second, Proxy: nil}
	defer t.CloseIdleConnections()
	client := &http.Client{Transport: t, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !authorized() {
				continue
			}
			statusData, _ := json.Marshal(m.info(peer))
			statusCtx, cancelStatus := context.WithTimeout(ctx, 6*time.Second)
			statusRequest, _ := http.NewRequestWithContext(statusCtx, http.MethodPost, "https://"+address+"/v2/status", bytes.NewReader(statusData))
			statusRequest.Header.Set("Content-Type", "application/json")
			statusOK := m.exchangeStatus(client, statusRequest, peer)
			cancelStatus()
			if !statusOK {
				continue
			}
			m.pollDiagnostic(ctx, client, address, peer, authorized)
			m.pollClipboard(ctx, client, address, peer, authorized)
			if offer := m.offered(peer); offer != nil {
				progress := &progressReader{Reader: bytes.NewReader(offer.data), update: func(n int64) { m.progress(peer, offer.id, "SENDING", n, int64(len(offer.data))) }}
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+address+"/v1/bundle", progress)
				if err != nil {
					continue
				}
				req.Header.Set("X-Krasis-ID", offer.id)
				req.Header.Set("X-Krasis-SHA256", offer.hash)
				req.Header.Set("Content-Type", "application/zip")
				resp, err := client.Do(req)
				state := "TRANSFER_UNKNOWN"
				if err == nil {
					if resp.StatusCode == http.StatusOK {
						state = "DELIVERED"
					} else if resp.StatusCode == http.StatusForbidden {
						state = "PEER_NOT_ALLOWED"
					} else {
						state = wireResult(resp.Header.Get("X-Krasis-Result"))
					}
					resp.Body.Close()
				}
				m.sent(offer.id, state)
				continue
			}
			if !m.permitted(peer) {
				continue
			}
			if status := m.Status(); status.State == "STAGED" && status.Peer == peer {
				ack, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+address+"/v1/receipt", nil)
				ack.Header.Set("X-Krasis-ID", status.ID)
				ack.Header.Set("X-Krasis-Result", "DELIVERED")
				if response, e := client.Do(ack); e == nil {
					if response.StatusCode == http.StatusOK && authorized() {
						m.ready(status.ID)
					}
					response.Body.Close()
				}
				continue
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+address+"/v1/bundle", nil)
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}
			id := resp.Header.Get("X-Krasis-ID")
			err = m.receive(peer, id, resp.Header.Get("X-Krasis-SHA256"), resp.Body, authorized)
			resp.Body.Close()
			if !validID(id, 32) {
				continue
			}
			ack, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+address+"/v1/receipt", nil)
			ack.Header.Set("X-Krasis-ID", id)
			if err == nil {
				ack.Header.Set("X-Krasis-Result", "DELIVERED")
			} else {
				ack.Header.Set("X-Krasis-Result", FailureCode(err))
			}
			if r, e := client.Do(ack); e == nil {
				if r.StatusCode == http.StatusOK && err == nil && authorized() {
					m.ready(id)
				}
				r.Body.Close()
			}
		}
	}
}

func (m *Manager) handler(peer string, authorized func() bool) http.Handler {
	gate := make(chan struct{}, 1)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		// TLS config verifies the one pinned certificate. Recheck current trust
		// on every request, not just the handshake. Input state is independent.
		if !authorized() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		select {
		case gate <- struct{}{}:
			defer func() { <-gate }()
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if r.URL.RawQuery != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch {
		case r.URL.Path == "/v1/clipboard":
			m.clipboardHTTP(w, r, peer, authorized)
		case r.URL.Path == "/v1/diagnostic-transfer":
			m.diagnosticHTTP(w, r, peer, authorized)
		case r.URL.Path == "/v2/status" && r.Method == http.MethodPost:
			if r.Header.Get("Content-Type") != "application/json" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			info, err := readPeerInfo(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			m.observePeer(peer, info)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(m.info(peer))
		case r.URL.Path == "/v1/bundle" && r.Method == http.MethodGet:
			if r.ContentLength != 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			offer := m.offered(peer)
			if offer == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("X-Krasis-ID", offer.id)
			w.Header().Set("X-Krasis-SHA256", offer.hash)
			w.Header().Set("Content-Length", strconv.Itoa(len(offer.data)))
			progress := &progressReader{Reader: bytes.NewReader(offer.data), update: func(n int64) { m.progress(peer, offer.id, "SENDING", n, int64(len(offer.data))) }}
			_, _ = io.Copy(w, progress)
		case r.URL.Path == "/v1/bundle" && r.Method == http.MethodPost:
			if r.Header.Get("Content-Type") != "application/zip" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			err := m.receive(peer, r.Header.Get("X-Krasis-ID"), r.Header.Get("X-Krasis-SHA256"), r.Body, authorized)
			if err != nil {
				w.Header().Set("X-Krasis-Result", FailureCode(err))
			}
			if errors.Is(err, ErrDenied) {
				w.WriteHeader(http.StatusForbidden)
			} else if errors.Is(err, ErrBusy) {
				w.WriteHeader(http.StatusConflict)
			} else if err != nil {
				w.WriteHeader(http.StatusUnprocessableEntity)
			} else {
				w.WriteHeader(http.StatusOK)
				if http.NewResponseController(w).Flush() == nil && authorized() {
					m.ready(r.Header.Get("X-Krasis-ID"))
				}
			}
		case r.URL.Path == "/v1/receipt" && r.Method == http.MethodPost:
			if r.ContentLength != 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			result := r.Header.Get("X-Krasis-Result")
			if result != wireResult(result) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			m.sent(r.Header.Get("X-Krasis-ID"), result)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
