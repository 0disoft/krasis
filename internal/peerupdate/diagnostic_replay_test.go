package peerupdate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type diagnosticFaultTransport struct {
	base       http.RoundTripper
	push, lost bool
}

func (f *diagnosticFaultTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !f.lost && !f.push && r.Method == http.MethodPut && r.URL.Path == "/v1/diagnostic-transfer" {
		f.lost = true
		return nil, errors.New("injected lost acknowledgement")
	}
	response, err := f.base.RoundTrip(r)
	if !f.lost && f.push && r.Method == http.MethodPost && r.URL.Path == "/v1/diagnostic-transfer" && err == nil && response.StatusCode == http.StatusOK {
		response.Body.Close()
		f.lost = true
		return nil, errors.New("injected lost success response")
	}
	return response, err
}

func TestDiagnosticCollectionRecoversLostAcknowledgementInBothRoles(t *testing.T) {
	for _, requesterListens := range []bool{true, false} {
		t.Run(fmt.Sprintf("requester_listens_%t", requesterListens), func(t *testing.T) {
			a, b := testManager(t), testManager(t)
			fpA, fpB := strings.Repeat("a", 64), strings.Repeat("b", 64)
			id, err := a.QueueDiagnosticRequest(fpB, "local.json")
			if err != nil {
				t.Fatal(err)
			}
			saves := 0
			a.ReceiveCollection = func(peer, requestID string, data []byte) (string, error) {
				if peer != fpB || requestID != id || string(data) != `{}` {
					t.Fatal("unexpected response identity")
				}
				saves++
				return "peer.json", nil
			}
			if err = b.QueueDiagnosticForRequest(fpA, id, []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
			var handler http.Handler
			if requesterListens {
				handler = a.handler(fpB, func() bool { return true })
			} else {
				handler = b.handler(fpA, func() bool { return true })
			}
			server := httptest.NewTLSServer(handler)
			defer server.Close()
			client := server.Client()
			fault := &diagnosticFaultTransport{base: client.Transport, push: requesterListens}
			client.Transport = fault
			address := strings.TrimPrefix(server.URL, "https://")
			poll := func() {
				if requesterListens {
					b.pollDiagnostic(context.Background(), client, address, fpA, func() bool { return true })
				} else {
					a.pollDiagnostic(context.Background(), client, address, fpB, func() bool { return true })
				}
			}
			poll()
			if !fault.lost || b.Status().Diagnostic != "QUEUED" {
				t.Fatal("loss did not leave sender awaiting acknowledgement")
			}
			poll()
			state, local, remote := a.DiagnosticRequestStatus(fpB, id)
			if state != "RECEIVED" || local != "local.json" || remote != "peer.json" || saves != 1 || b.Status().Diagnostic != "SENT" {
				t.Fatalf("state=%s local=%s remote=%s saves=%d sender=%s", state, local, remote, saves, b.Status().Diagnostic)
			}
		})
	}
}

func TestDiagnosticReplayRequiresExactSavedIdentityContentAndAuthorization(t *testing.T) {
	m := testManager(t)
	peer, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	requestID, err := m.QueueDiagnosticRequest(peer, "local.json")
	if err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("c", 32)
	saves := 0
	m.ReceiveCollection = func(string, string, []byte) (string, error) { saves++; return "peer.json", nil }
	if err = m.receiveDiagnostic(peer, id, requestID, strings.NewReader(`{}`), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	// A later collection must neither invalidate the saved acknowledgement nor
	// be completed by a replay of the previous collection.
	newID, err := m.QueueDiagnosticRequest(other, "new-local.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.receiveDiagnostic(peer, id, requestID, strings.NewReader(`{}`), func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		peer, id, requestID, data string
		authorized                bool
	}{
		{peer, id, requestID, `{}`, false},
		{other, id, requestID, `{}`, true},
		{peer, strings.Repeat("d", 32), requestID, `{}`, true},
		{peer, id, strings.Repeat("d", 32), `{}`, true},
		{peer, id, "", `{}`, true},
		{peer, id, requestID, `{"changed":true}`, true},
	} {
		if err = m.receiveDiagnostic(test.peer, test.id, test.requestID, strings.NewReader(test.data), func() bool { return test.authorized }); !errors.Is(err, ErrDenied) {
			t.Fatal("conflicting replay accepted", test, err)
		}
	}
	if state, local, remote := m.DiagnosticRequestStatus(other, newID); state != "PENDING" || local != "new-local.json" || remote != "" || saves != 1 {
		t.Fatal(state, local, remote, saves)
	}
	old := m.diagnostic.last[peer]
	old.expires = time.Now().Add(-time.Second)
	m.diagnostic.last[peer] = old
	if err = m.receiveDiagnostic(peer, id, requestID, strings.NewReader(`{}`), func() bool { return true }); !errors.Is(err, ErrDenied) {
		t.Fatal("expired collection receipt accepted", err)
	}
}

func TestDiagnosticReceiptCapacityReclaimsExpiredPeers(t *testing.T) {
	m := testManager(t)
	m.ReceiveDiagnostic = func(string, []byte) (string, error) { return "saved.json", nil }
	for i := 1; i <= 32; i++ {
		if err := m.receiveDiagnostic(fmt.Sprintf("%064x", i), fmt.Sprintf("%032x", i), "", strings.NewReader(`{}`), func() bool { return true }); err != nil {
			t.Fatal(err)
		}
	}
	peer, id := fmt.Sprintf("%064x", 33), fmt.Sprintf("%032x", 33)
	if err := m.receiveDiagnostic(peer, id, "", strings.NewReader(`{}`), func() bool { return true }); !errors.Is(err, ErrBusy) {
		t.Fatal("receipt capacity not enforced", err)
	}
	first := fmt.Sprintf("%064x", 1)
	old := m.diagnostic.last[first]
	old.expires = time.Now().Add(-time.Second)
	m.diagnostic.last[first] = old
	if err := m.receiveDiagnostic(peer, id, "", strings.NewReader(`{}`), func() bool { return true }); err != nil || len(m.diagnostic.last) != 32 {
		t.Fatal("expired slot not reclaimed", err)
	}
}

func TestDiagnosticSaveFailureCreatesNoAcknowledgement(t *testing.T) {
	for _, fail := range []bool{true, false} {
		m := testManager(t)
		peer := strings.Repeat("a", 64)
		requestID, err := m.QueueDiagnosticRequest(peer, "local.json")
		if err != nil {
			t.Fatal(err)
		}
		m.ReceiveCollection = func(string, string, []byte) (string, error) {
			if fail {
				return "", errors.New("save failed")
			}
			return "", nil
		}
		if err = m.receiveDiagnostic(peer, strings.Repeat("b", 32), requestID, strings.NewReader(`{}`), func() bool { return true }); err == nil {
			t.Fatal("missing saved file acknowledged")
		}
		if state, _, remote := m.DiagnosticRequestStatus(peer, requestID); state != "PENDING" || remote != "" || len(m.diagnostic.last) != 0 {
			t.Fatal(state, remote)
		}
	}
}
