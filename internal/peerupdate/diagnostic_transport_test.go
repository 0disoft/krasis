package peerupdate

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunIdleDiagnosticPollReusesTLSConnection(t *testing.T) {
	local, remote := testManager(t), testManager(t)
	peer, caller := strings.Repeat("a", 64), strings.Repeat("b", 64)
	local.ExportDiagnostic = func(string, string) error { return nil }
	local.observePeer(peer, PeerInfo{})
	handler := remote.handler(caller, func() bool { return true })
	var mu sync.Mutex
	connections := make(map[string]bool)
	requests := make(map[string]int)
	polled := make(chan struct{})
	var polledOnce sync.Once
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
		if r.URL.Path != "/v1/diagnostic-transfer" && r.URL.Path != "/v1/diagnostic-request" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		connections[r.RemoteAddr] = true
		requests[r.URL.Path]++
		if len(requests) == 2 {
			polledOnce.Do(func() { close(polled) })
		}
	}))
	defer server.Close()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		local.Run(ctx, net.JoinHostPort(host, strconv.Itoa(n-1)), peer, false,
			server.Client().Transport.(*http.Transport).TLSClientConfig, func() bool { return true })
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("management workers failed to stop")
		}
	}()
	select {
	case <-polled:
	case <-time.After(7 * time.Second):
		t.Fatal("idle diagnostic poll did not finish")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(connections) != 1 || requests["/v1/diagnostic-transfer"] != 1 || requests["/v1/diagnostic-request"] != 1 {
		t.Fatalf("idle diagnostic batch: TLS connections=%d, requests=%v; want 1 connection for 2 requests", len(connections), requests)
	}
}
