package peerupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestAppOpenWaitsForWindowReportWithoutRepeatingLaunch(t *testing.T) {
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	launches, ready := 0, false
	m := &Manager{OpenUI: func(string) string { launches++; return "launch_requested" }, UIReady: func(time.Time) bool { return ready }}
	allowed := func() bool { return true }
	if m.acceptAppOpen(peer, id, allowed) != "launch_requested" {
		t.Fatal("premature completion")
	}
	ready = true
	if m.acceptAppOpen(peer, id, allowed) != "window_ready" || launches != 1 {
		t.Fatal("window not observed or repeated launch")
	}
	sender := &Manager{}
	queued, _ := sender.QueueAppOpen(peer)
	sender.finishAppOpen(peer, queued, "launch_requested")
	if sender.appOpenOffer(peer) != queued {
		t.Fatal("stopped polling before window ready")
	}
	sender.finishAppOpen(peer, queued, "window_ready")
	if sender.AppOpenStatus(peer, queued) != "window_ready" {
		t.Fatal("window completion lost")
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

func TestAppOpenNoResponseStopsLateDeliveryWithoutStatusPolling(t *testing.T) {
	peer := strings.Repeat("a", 64)
	for _, check := range []string{"offer", "reply", "queue"} {
		t.Run(check, func(t *testing.T) {
			m := &Manager{}
			id, _ := m.QueueAppOpen(peer)
			m.appOpen.out.respondBy = time.Now().Add(-time.Second)
			switch check {
			case "offer":
				if m.appOpenOffer(peer) != "" {
					t.Fatal("late request was advertised")
				}
			case "reply":
				m.finishAppOpen(peer, id, "launch_requested")
			case "queue":
				if next, err := m.QueueAppOpen(peer); err != nil || next == id {
					t.Fatal("expired request blocked a new attempt", err)
				}
				m.finishAppOpen(peer, id, "window_ready")
				if m.appOpen.out.code != "pending" {
					t.Fatal("old receipt changed a new request")
				}
				return
			}
			if m.AppOpenStatus(peer, id) != "peer_unresponsive" {
				t.Fatal("late reply changed the terminal result")
			}
		})
	}
}

func TestAppOpenAcknowledgedLaunchKeepsWindowDeadline(t *testing.T) {
	peer := strings.Repeat("a", 64)
	m := &Manager{}
	id, _ := m.QueueAppOpen(peer)
	m.finishAppOpen(peer, id, "launch_requested")
	m.appOpen.out.respondBy = time.Now().Add(-time.Second)
	if m.appOpenOffer(peer) != id || m.AppOpenStatus(peer, id) != "launch_requested" {
		t.Fatal("acknowledged launch lost the window-readiness interval")
	}
	m.appOpen.out.expires = time.Now().Add(-time.Second)
	m.finishAppOpen(peer, id, "window_ready")
	if m.appOpenOffer(peer) != "" || m.AppOpenStatus(peer, id) != "window_unconfirmed" {
		t.Fatal("window deadline was not enforced")
	}
}
