package peerupdate

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestExpiredDiagnosticRetrySurvivesStatusPolling(t *testing.T) {
	for _, polls := range []int{0, 1, 3} {
		t.Run(fmt.Sprintf("status_polls_%d", polls), func(t *testing.T) {
			m := testManager(t)
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			if err := m.QueueDiagnosticForRequest(peer, id, []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
			m.diagnostic.mu.Lock()
			m.diagnostic.out.expires = time.Now().Add(-time.Second)
			m.diagnostic.mu.Unlock()
			for range polls {
				if state := m.Status().Diagnostic; state != "EXPIRED" {
					t.Fatal("expired response was not reported", state)
				}
			}
			exports := 0
			m.ExportDiagnostic = func(p, request string) error {
				exports++
				return m.QueueDiagnosticForRequest(p, request, []byte(`{}`))
			}
			for range 2 {
				if err := m.runDiagnosticExport(context.Background(), peer, id, func() bool { return true }); err != nil {
					t.Fatal(err)
				}
			}
			if exports != 1 || m.diagnosticOffer(peer) == nil {
				t.Fatalf("status polling suppressed an unsent retry: exports=%d, want 1 queued response", exports)
			}
		})
	}
}

func TestManualDiagnosticReplacementReleasesExpiredRequest(t *testing.T) {
	m := testManager(t)
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	if err := m.QueueDiagnosticForRequest(peer, id, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	m.diagnostic.mu.Lock()
	m.diagnostic.out.expires = time.Now().Add(-time.Second)
	m.diagnostic.mu.Unlock()
	if err := m.QueueDiagnostic(peer, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if m.DiagnosticRequestHandled(peer, id) {
		t.Fatal("manual replacement retained the expired request's deduplication record")
	}
}

func TestDiagnosticExpiryPreservesConfirmedAndNewerReceipts(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		t.Run(fmt.Sprintf("confirmed_%t", confirmed), func(t *testing.T) {
			m := testManager(t)
			peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
			if err := m.QueueDiagnosticForRequest(peer, id, []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
			if confirmed {
				offer := m.diagnosticOffer(peer)
				m.diagnosticDone(offer.id, "SENT")
			} else {
				id = strings.Repeat("c", 32)
				m.diagnostic.mu.Lock()
				m.diagnostic.handled[peer] = id
				m.diagnostic.out.expires = time.Now().Add(-time.Second)
				m.diagnostic.mu.Unlock()
			}
			m.Status()
			if !m.DiagnosticRequestHandled(peer, id) {
				t.Fatal("expiry removed a confirmed or newer request's receipt")
			}
		})
	}
}
