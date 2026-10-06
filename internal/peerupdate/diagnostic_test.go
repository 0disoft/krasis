package peerupdate

import (
	"context"
	"errors"
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

func TestDiagnosticCollectionReturnsPeerExportInBothRoles(t *testing.T) {
	for _, requesterListens := range []bool{true, false} {
		t.Run(map[bool]string{true: "requester_listens", false: "peer_listens"}[requesterListens], func(t *testing.T) {
			a, b := testManager(t), testManager(t)
			fpA, fpB := strings.Repeat("a", 64), strings.Repeat("b", 64)
			id, err := a.QueueDiagnosticRequest(fpB, "local.json")
			if err != nil {
				t.Fatal(err)
			}
			exports, receives := 0, 0
			b.ExportDiagnostic = func(peer, requestID string) error {
				if peer != fpA || requestID != id {
					t.Fatal("request crossed peer or ID")
				}
				if b.DiagnosticRequestHandled(peer, requestID) {
					return nil
				}
				exports++
				return b.QueueDiagnosticForRequest(peer, requestID, []byte(`{}`))
			}
			a.ReceiveCollection = func(peer, requestID string, data []byte) (string, error) {
				if peer != fpB || requestID != id || string(data) != `{}` {
					t.Fatal("response crossed peer")
				}
				receives++
				return "peer.json", nil
			}
			if requesterListens {
				server := httptest.NewTLSServer(a.handler(fpB, func() bool { return true }))
				defer server.Close()
				address := strings.TrimPrefix(server.URL, "https://")
				b.pollDiagnosticRequest(context.Background(), server.Client(), address, fpA, func() bool { return true })
				b.pollDiagnosticRequest(context.Background(), server.Client(), address, fpA, func() bool { return true })
				b.pollDiagnostic(context.Background(), server.Client(), address, fpA, func() bool { return true })
			} else {
				server := httptest.NewTLSServer(b.handler(fpA, func() bool { return true }))
				defer server.Close()
				address := strings.TrimPrefix(server.URL, "https://")
				a.pushDiagnosticRequest(context.Background(), server.Client(), address, fpB, func() bool { return true })
				a.pushDiagnosticRequest(context.Background(), server.Client(), address, fpB, func() bool { return true })
				a.pollDiagnostic(context.Background(), server.Client(), address, fpB, func() bool { return true })
			}
			state, local, remote := a.DiagnosticRequestStatus(fpB, id)
			if state != "RECEIVED" || local != "local.json" || remote != "peer.json" || exports != 1 || receives != 1 {
				t.Fatalf("state=%s local=%s remote=%s exports=%d receives=%d", state, local, remote, exports, receives)
			}
		})
	}
}

func TestDiagnosticCollectionPublishesOnlyAfterSnapshotAndSkipsBusyExports(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	fault := errors.New("snapshot save failed")
	if _, err := m.PrepareDiagnosticRequest(peer, func(string) (string, error) { return "", fault }); !errors.Is(err, fault) {
		t.Fatal(err)
	}
	if m.diagnosticRequestOffer(peer) != "" {
		t.Fatal("failed snapshot advertised")
	}
	var savedID string
	id, err := m.PrepareDiagnosticRequest(peer, func(id string) (string, error) { savedID = id; return id + ".json", nil })
	if err != nil || id != savedID {
		t.Fatal(id, savedID, err)
	}
	if state, local, _ := m.DiagnosticRequestStatus(peer, id); state != "PENDING" || local != id+".json" {
		t.Fatal(state, local)
	}
	if _, err = m.PrepareDiagnosticRequest(peer, func(string) (string, error) { t.Fatal("busy request exported again"); return "", nil }); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
}

func TestDiagnosticCollectionRejectsUnmatchedResponseAndRevokedPeer(t *testing.T) {
	a := testManager(t)
	fp := strings.Repeat("b", 64)
	id, err := a.QueueDiagnosticRequest(fp, "local.json")
	if err != nil {
		t.Fatal(err)
	}
	a.ReceiveDiagnostic = func(string, []byte) (string, error) { t.Fatal("unmatched response stored"); return "", nil }
	if err := a.receiveDiagnostic(fp, strings.Repeat("c", 32), strings.Repeat("d", 32), strings.NewReader(`{}`), func() bool { return true }); err == nil {
		t.Fatal("unmatched response accepted")
	}
	if state, _, _ := a.DiagnosticRequestStatus(fp, id); state != "PENDING" {
		t.Fatal(state)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/diagnostic-request", nil)
	w := httptest.NewRecorder()
	a.handler(fp, func() bool { return false }).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || w.Header().Get("X-Krasis-Request-ID") != "" {
		t.Fatalf("revoked peer received request: %d", w.Code)
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
