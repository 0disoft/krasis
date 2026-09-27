package peerupdate

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiagnosticPeerBindingAndReceipt(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	other := strings.Repeat("b", 64)
	if err := m.QueueDiagnostic(peer, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if m.diagnosticOffer(other) != nil {
		t.Fatal("cross-peer disclosure")
	}
	offer := m.diagnosticOffer(peer)
	m.diagnosticDone("wrong", "SENT")
	if m.diagnosticOffer(peer) == nil {
		t.Fatal("wrong receipt consumed offer")
	}
	m.diagnosticDone(offer.id, "SENT")
	if m.Status().Diagnostic != "SENT" {
		t.Fatal("receipt missing")
	}
}
func TestDiagnosticQueueAcceptsExtendedSupport(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	data := []byte(strings.Repeat("x", (1<<20)+1))
	if err := m.QueueDiagnostic(peer, data); err != nil {
		t.Fatalf("extended support rejected: %v", err)
	}
	if offer := m.diagnosticOffer(peer); offer == nil || len(offer.data) != len(data) {
		t.Fatal("extended support offer missing")
	}
	if err := m.QueueDiagnostic(peer, make([]byte, MaxDiagnosticBytes+1)); err == nil {
		t.Fatal("oversized support accepted")
	}
}
func TestDiagnosticRouteChecksTrustAndDeduplicates(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	id := strings.Repeat("c", 32)
	calls := 0
	m.ReceiveDiagnostic = func(string, []byte) (string, error) { calls++; return "received.json", nil }
	for _, allowed := range []bool{false, true, true} {
		req := httptest.NewRequest(http.MethodPost, "/v1/diagnostic-transfer", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Krasis-ID", id)
		w := httptest.NewRecorder()
		m.handler(peer, func() bool { return allowed }).ServeHTTP(w, req)
		if !allowed && w.Code != 403 {
			t.Fatal("trust bypass")
		}
		if allowed && w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	if calls != 1 {
		t.Fatalf("duplicate stored %d times", calls)
	}
}
