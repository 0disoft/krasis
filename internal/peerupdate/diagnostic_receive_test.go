package peerupdate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBusyDiagnosticReceiverKeepsSameOutgoingTransfer(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	if err := m.QueueDiagnostic(peer, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	first := m.diagnosticOffer(peer)
	sent := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			sent++
			if r.Header.Get("X-Krasis-ID") != first.id {
				t.Error("busy retry changed transfer identity")
			}
			if sent == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "https://")
	m.pollDiagnostic(context.Background(), server.Client(), address, peer, func() bool { return true })
	current := m.diagnosticOffer(peer)
	if current == nil || current.id != first.id || !current.expires.Equal(first.expires) {
		t.Fatal("busy receiver discarded or extended transfer")
	}
	m.pollDiagnostic(context.Background(), server.Client(), address, peer, func() bool { return true })
	if sent != 2 || m.Status().Diagnostic != "SENT" {
		t.Fatal(sent, m.Status().Diagnostic)
	}
}

func TestDiagnosticReceiveSaveLeavesStatusResponsive(t *testing.T) {
	m := testManager(t)
	peer, transfer := strings.Repeat("a", 64), strings.Repeat("b", 32)
	request, err := m.QueueDiagnosticRequest(peer, "local.json")
	if err != nil {
		t.Fatal(err)
	}
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	defer close(release)
	m.ReceiveCollection = func(string, string, []byte) (string, error) { close(started); <-release; return "peer.json", nil }
	go func() {
		done <- m.receiveDiagnostic(peer, transfer, request, strings.NewReader(`{}`), func() bool { return true })
	}()
	<-started
	checked := make(chan error, 1)
	go func() {
		m.Status()
		state, _, _ := m.DiagnosticRequestStatus(peer, request)
		if state != "PENDING" {
			checked <- errors.New("save announced before completion")
			return
		}
		_, err := m.QueueDiagnosticRequest(peer, "other.json")
		if !errors.Is(err, ErrBusy) {
			checked <- errors.New("request replaced while saving")
			return
		}
		err = m.receiveDiagnostic(peer, transfer, request, strings.NewReader(`{}`), func() bool { return true })
		if !errors.Is(err, ErrBusy) {
			checked <- errors.New("duplicate concurrent save accepted")
			return
		}
		checked <- nil
	}()
	select {
	case err = <-checked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("disk save blocked status")
	}
	release <- struct{}{}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	state, _, path := m.DiagnosticRequestStatus(peer, request)
	if state != "RECEIVED" || path != "peer.json" {
		t.Fatal(state, path)
	}
}

func TestDiagnosticReceiveChecksAuthorizationAfterSaveAndReleasesSlot(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	allowed := true
	m.ReceiveDiagnostic = func(string, []byte) (string, error) { allowed = false; m.Status(); return "peer.json", nil }
	err := m.receiveDiagnostic(peer, strings.Repeat("b", 32), "", strings.NewReader(`{}`), func() bool { return allowed })
	if !errors.Is(err, ErrDenied) || m.diagnostic.receiving || len(m.diagnostic.last) != 0 {
		t.Fatal("revoked save acknowledged or slot stuck", err)
	}
	m.ReceiveDiagnostic = func(string, []byte) (string, error) { return "peer.json", nil }
	if err = m.receiveDiagnostic(peer, strings.Repeat("c", 32), "", strings.NewReader(`{}`), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
}
