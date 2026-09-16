package peerupdate

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	base := t.TempDir()
	if _, err := peercredentials.ProtectSecurityDirectory(base); err != nil {
		t.Fatal(err)
	}
	m, err := Open(base, 21)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}
func TestUpdatePermissionIsExplicitPersistedAndPeerScoped(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	other := strings.Repeat("b", 64)
	if m.permitted(peer) {
		t.Fatal("default allow")
	}
	if err := m.Permit(peer, true); err != nil {
		t.Fatal(err)
	}
	if !m.permitted(peer) || m.permitted(other) {
		t.Fatal("permission crossed peers")
	}
	data, err := os.ReadFile(m.path + "/permissions.json")
	if err != nil || !bytes.Contains(data, []byte(peer)) {
		t.Fatalf("persist: %v", err)
	}
	m.status = Status{State: "READY", Peer: peer, ID: strings.Repeat("c", 32)}
	if err := m.Permit(peer, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Claim(strings.Repeat("c", 32), peer); err == nil {
		t.Fatal("revocation did not fence apply")
	}
}
func TestUpdateClaimIsSingleUseAndKeepsPeerBinding(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	id := strings.Repeat("c", 32)
	m.allowed[peer] = true
	m.status = Status{State: "READY", Peer: peer, ID: id, Revision: strings.Repeat("d", 40)}
	if _, _, err := m.Claim(id, strings.Repeat("b", 64)); err == nil {
		t.Fatal("wrong peer accepted")
	}
	if _, _, err := m.Claim(id, peer); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Claim(id, peer); err == nil {
		t.Fatal("replay accepted")
	}
	if err := m.Permit(peer, false); err != ErrBusy {
		t.Fatal("apply authority mutated after commit")
	}
}
func TestUpdateReceiveDeniesBeforeReadingBody(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	for _, authorize := range []bool{false, true} {
		if err := m.receive(peer, strings.Repeat("b", 32), strings.Repeat("c", 64), panicReader{}, func() bool { return authorize }); err != ErrDenied {
			t.Fatalf("receive: %v", err)
		}
	}
	if err := m.Permit(peer, true); err != nil {
		t.Fatal(err)
	}
	if err := m.receive(peer, strings.Repeat("b", 32), strings.Repeat("c", 64), strings.NewReader("corrupt"), func() bool { return true }); err == nil {
		t.Fatal("corrupt archive accepted")
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("unauthorized body read") }

func TestAuxiliaryRoutesRecheckAuthorization(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	for _, path := range []string{"/v1/bundle", "/v1/receipt"} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(method, path, nil)
			m.handler(peer, func() bool { return false }).ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s %s bypassed auth", method, path)
			}
		}
	}
}

type fakeBundle struct {
	generation int
	staged     bool
}

func (b *fakeBundle) Metadata() updatebundle.Metadata {
	return updatebundle.Metadata{Generation: b.generation, Revision: strings.Repeat("d", 40)}
}
func (b *fakeBundle) Stage(*os.Root, string) error { b.staged = true; return nil }
func TestReceiveRechecksPermissionAndGenerationBeforeStaging(t *testing.T) {
	for _, scenario := range []string{"success", "revoked", "stale", "connection_lost"} {
		t.Run(scenario, func(t *testing.T) {
			m := testManager(t)
			peer := strings.Repeat("a", 64)
			id := strings.Repeat("b", 32)
			if err := m.Permit(peer, true); err != nil {
				t.Fatal(err)
			}
			b := &fakeBundle{generation: 22}
			connected := true
			m.validate = func([]byte) (stagedBundle, error) {
				switch scenario {
				case "revoked":
					if err := m.Permit(peer, false); err != nil {
						t.Fatal(err)
					}
				case "stale":
					b.generation = 21
				case "connection_lost":
					connected = false
				}
				return b, nil
			}
			data := []byte("verified by fake validator")
			err := m.receive(peer, id, fmt.Sprintf("%x", sha256.Sum256(data)), bytes.NewReader(data), func() bool { return connected })
			if (err == nil) != (scenario == "success") || b.staged != (scenario == "success") {
				t.Fatalf("result: %v staged=%t", err, b.staged)
			}
			if err == nil {
				if m.Status().State != "STAGED" {
					t.Fatal("applied before receipt")
				}
				m.ready(id)
				if m.Status().State != "READY" {
					t.Fatal("missing ready state")
				}
			}
		})
	}
}

func TestUploadOnlyBecomesReadyAfterResponseFlush(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	id := strings.Repeat("b", 32)
	if err := m.Permit(peer, true); err != nil {
		t.Fatal(err)
	}
	b := &fakeBundle{generation: 22}
	m.validate = func([]byte) (stagedBundle, error) { return b, nil }
	data := []byte("bounded fixture")
	r := httptest.NewRequest(http.MethodPost, "/v1/bundle", bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/zip")
	r.Header.Set("X-Krasis-ID", id)
	r.Header.Set("X-Krasis-SHA256", fmt.Sprintf("%x", sha256.Sum256(data)))
	w := httptest.NewRecorder()
	m.handler(peer, func() bool { return true }).ServeHTTP(w, r)
	if w.Code != http.StatusOK || !w.Flushed || m.Status().State != "READY" {
		t.Fatalf("code=%d flushed=%t state=%s", w.Code, w.Flushed, m.Status().State)
	}
}

func TestDownloadReceiptOnlyCompletesExactQueuedOffer(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	id := strings.Repeat("b", 32)
	m.out = &offer{id: id, peer: peer, data: []byte("fixture"), expires: time.Now().Add(time.Minute)}
	m.status = Status{State: "QUEUED", ID: id, Peer: peer}
	for _, ack := range []string{strings.Repeat("c", 32), id} {
		r := httptest.NewRequest(http.MethodPost, "/v1/receipt", nil)
		r.Header.Set("X-Krasis-ID", ack)
		r.Header.Set("X-Krasis-Result", "DELIVERED")
		w := httptest.NewRecorder()
		m.handler(peer, func() bool { return true }).ServeHTTP(w, r)
		if (m.Status().State == "DELIVERED") != (ack == id) {
			t.Fatal("receipt changed wrong offer")
		}
	}
}
