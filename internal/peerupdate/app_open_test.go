package peerupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppOpenDeduplicatesAndRechecksAuthorization(t *testing.T) {
	peer := strings.Repeat("a", 64)
	id := strings.Repeat("b", 32)
	count, allowed := 0, true
	m := &Manager{OpenUI: func(string) string { count++; return "launch_requested" }}
	gate := func() bool { return allowed }
	for i := 0; i < 2; i++ {
		if code := m.acceptAppOpen(peer, id, gate); code != "launch_requested" {
			t.Fatal(code)
		}
	}
	if count != 1 {
		t.Fatal("repeated launch")
	}
	if code := m.acceptAppOpen(peer, strings.Repeat("c", 32), gate); code != "busy" {
		t.Fatal(code)
	}
	allowed = false
	if code := m.acceptAppOpen(peer, id, gate); code != "rejected" || count != 1 {
		t.Fatal("revoked request accepted")
	}
	if code := m.acceptAppOpen(peer, "bad", func() bool { return true }); code != "rejected" {
		t.Fatal(code)
	}
}

func TestAppOpenBothManagementDirections(t *testing.T) {
	peer := strings.Repeat("a", 64)
	for _, senderIsListener := range []bool{false, true} {
		count := 0
		sender, receiver := &Manager{}, &Manager{OpenUI: func(string) string { count++; return "launch_requested" }}
		id, err := sender.QueueAppOpen(peer)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sender.QueueAppOpen(peer); err != ErrBusy {
			t.Fatal("pending request replaced")
		}
		listener, connector := receiver, sender
		if senderIsListener {
			listener, connector = sender, receiver
		}
		server := httptest.NewTLSServer(listener.handler(peer, func() bool { return true }))
		connector.pollAppOpen(context.Background(), server.Client(), strings.TrimPrefix(server.URL, "https://"), peer, func() bool { return true })
		server.Close()
		if code := sender.AppOpenStatus(peer, id); code != "launch_requested" || count != 1 {
			t.Fatalf("listener sender=%v code=%s count=%d", senderIsListener, code, count)
		}
		if code := sender.AppOpenStatus(strings.Repeat("d", 64), id); code != "unknown_request" {
			t.Fatal("cross-peer receipt")
		}
	}
}

func TestAppOpenRejectsPayloadAndArbitraryResult(t *testing.T) {
	m := &Manager{}
	peer := strings.Repeat("a", 64)
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/v1/app-open", strings.NewReader(`{"path":"other.exe"}`)),
		httptest.NewRequest(http.MethodPut, "/v1/app-open", nil),
	} {
		request.Header.Set("X-Krasis-Request-ID", strings.Repeat("b", 32))
		request.Header.Set("X-Krasis-Result", "arbitrary")
		response := httptest.NewRecorder()
		m.appOpenHTTP(response, request, peer, func() bool { return true })
		if response.Code != http.StatusBadRequest {
			t.Fatal(response.Code)
		}
	}
}
