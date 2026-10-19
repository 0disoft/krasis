package peerupdate

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestControlConnectionReuseStillChecksTrustAndKeepsBulkIndependent(t *testing.T) {
	m := testManager(t)
	var allowed atomic.Bool
	allowed.Store(true)
	var connections, checks atomic.Int32
	server := httptest.NewUnstartedServer(m.handler(strings.Repeat("a", 64), func() bool {
		checks.Add(1)
		return allowed.Load()
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.StartTLS()
	defer server.Close()
	base := &http.Transport{TLSClientConfig: server.Client().Transport.(*http.Transport).TLSClientConfig.Clone(), DisableKeepAlives: true, MaxConnsPerHost: 1}
	control := newControlTransport(base)
	defer control.CloseIdleConnections()
	defer base.CloseIdleConnections()
	client := &http.Client{Transport: control, Timeout: 3 * time.Second}
	bulk := &http.Client{Transport: base, Timeout: 3 * time.Second}
	get := func(c *http.Client, want int) {
		t.Helper()
		r, err := c.Get(server.URL + "/v1/app-open")
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != want {
			t.Fatal("unexpected control result", r.StatusCode)
		}
	}
	for range 3 {
		get(client, http.StatusNoContent)
	}
	if connections.Load() != 1 || checks.Load() != 3 {
		t.Fatal("immediate control requests did not reuse one authenticated connection", connections.Load(), checks.Load())
	}
	for range 2 {
		get(bulk, http.StatusNoContent)
	}
	if connections.Load() != 3 {
		t.Fatal("control reuse altered bulk transport behavior", connections.Load())
	}
	allowed.Store(false)
	get(client, http.StatusForbidden)
	if checks.Load() != 6 || connections.Load() != 3 {
		t.Fatal("reused connection bypassed current trust", checks.Load(), connections.Load())
	}
	control.CloseIdleConnections()
	get(client, http.StatusForbidden)
	if connections.Load() != 4 || checks.Load() != 7 {
		t.Fatal("idle control socket survived explicit closure", connections.Load(), checks.Load())
	}
}

func TestReusedControlConnectionDoesNotReplayAnAcceptedLaunchWithLostReply(t *testing.T) {
	var launches, handshakes atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		launches.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close() // Action accepted; no response is sent.
	}))
	server.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) { handshakes.Add(1); return nil, nil }}
	server.StartTLS()
	defer server.Close()
	base := &http.Transport{TLSClientConfig: server.Client().Transport.(*http.Transport).TLSClientConfig.Clone(), DisableKeepAlives: true, MaxConnsPerHost: 1}
	transport := newControlTransport(base)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	warm, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	warm.Body.Close()
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/app-open", nil)
	request.Header.Set("X-Krasis-Request-ID", strings.Repeat("b", 32))
	response, err := client.Do(request)
	if response != nil {
		response.Body.Close()
	}
	if err == nil || launches.Load() != 1 || handshakes.Load() != 1 {
		t.Fatal("uncertain accepted launch was retried or lost its failure", err, launches.Load(), handshakes.Load())
	}
}
