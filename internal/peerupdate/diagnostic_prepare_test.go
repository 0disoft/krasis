package peerupdate

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticSnapshotPreparationDoesNotHoldTransferLock(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	started, release := make(chan string, 1), make(chan struct{})
	defer close(release)
	done := make(chan error, 1)
	go func() {
		_, err := m.PrepareDiagnosticRequest(peer, func(id string) (string, error) {
			started <- id
			<-release
			return "local.json", nil
		})
		done <- err
	}()
	id := <-started
	checks := make(chan error, 1)
	go func() {
		if m.diagnosticRequestOffer(peer) != "" {
			checks <- errors.New("unfinished snapshot published")
			return
		}
		if state, _, _ := m.DiagnosticRequestStatus(peer, id); state != "UNKNOWN" {
			checks <- errors.New("preparation exposed as pending request")
			return
		}
		_, err := m.QueueDiagnosticRequest(peer, "other.json")
		if !errors.Is(err, ErrBusy) {
			checks <- errors.New("concurrent preparation accepted")
			return
		}
		checks <- m.QueueDiagnostic(peer, []byte(`{}`))
	}()
	select {
	case err := <-checks:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("snapshot save blocked unrelated transfer operations")
	}
	// Release without closing, so the deferred cleanup also handles early failure.
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if state, local, _ := m.DiagnosticRequestStatus(peer, id); state != "PENDING" || local != "local.json" {
		t.Fatal(state, local)
	}
}

func TestFailedDiagnosticPreparationPreservesPreviousRequestAndReleasesSlot(t *testing.T) {
	for _, fail := range []bool{true, false} {
		m := testManager(t)
		peer := strings.Repeat("a", 64)
		old := &diagnosticRequest{peer: peer, id: strings.Repeat("b", 32), state: "RECEIVED", localPath: "old-local.json", path: "old-peer.json"}
		m.diagnostic.request = old
		if _, err := m.PrepareDiagnosticRequest(peer, func(string) (string, error) {
			if fail {
				return "", errors.New("save failed")
			}
			return "", nil
		}); err == nil {
			t.Fatal("failed snapshot published")
		}
		if m.diagnostic.request != old || m.diagnostic.preparing {
			t.Fatal("previous result lost or slot stuck")
		}
		if _, err := m.PrepareDiagnosticRequest(peer, func(string) (string, error) {
			// Calling status inside the callback must also remain safe.
			m.DiagnosticRequestStatus(peer, old.id)
			return "new-local.json", nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
