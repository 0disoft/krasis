package peerupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestStatusThrottlePreservesQueuedArchiveUntilFreshPreflight(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	current := queuedFileOffer(t, m, peer)
	m.status.Generation, m.status.Revision = 24, strings.Repeat("b", 40)
	expires := current.expires
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(PeerInfo{Protocol: 2, Generation: 23, Allowed: true})
	}))
	defer server.Close()
	exchange := func() bool {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/v2/status", nil)
		if err != nil {
			t.Fatal(err)
		}
		return m.exchangeStatus(server.Client(), req, peer)
	}
	if exchange() {
		t.Fatal("throttled preflight authorized a transfer")
	}
	if m.out != current || current.retired || !current.expires.Equal(expires) || m.Status().State != "QUEUED" {
		t.Fatal("transient throttle discarded or extended the queued archive", m.Status().State)
	}
	if _, err := current.file.Stat(); err != nil {
		t.Fatal("throttle closed the queued archive", err)
	}
	if !exchange() || calls.Load() != 2 || m.out != current || current.retired {
		t.Fatal("fresh preflight could not resume the same archive", calls.Load())
	}
	reader := m.offered(peer)
	if reader == nil || reader.id != current.id {
		t.Fatal("retry lost archive identity")
	}
	reader.Close()
	// A later negative permission verdict must still retire the retained offer.
	m.observePeer(peer, PeerInfo{Protocol: 2, Generation: 23, Allowed: false})
	if m.out != nil || m.Status().State != "PEER_NOT_ALLOWED" {
		t.Fatal("revoked permission did not retire the queued archive")
	}
}

func TestTerminalStatusFailureStillRetiresQueuedArchive(t *testing.T) {
	for _, tc := range []struct {
		status int
		state  string
	}{{http.StatusForbidden, "PEER_NOT_ALLOWED"}, {http.StatusNotFound, "PEER_UNSUPPORTED"}, {http.StatusInternalServerError, "PEER_UNREACHABLE"}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			m := testManager(t)
			peer := strings.Repeat("a", 64)
			current := queuedFileOffer(t, m, peer)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status) }))
			defer server.Close()
			req, err := http.NewRequest(http.MethodPost, server.URL+"/v2/status", nil)
			if err != nil {
				t.Fatal(err)
			}
			if m.exchangeStatus(server.Client(), req, peer) || m.out != nil || m.Status().State != tc.state {
				t.Fatal("terminal status retained the archive", tc.status, m.Status().State)
			}
			if _, err := current.file.ReadAt(make([]byte, 1), 0); !errors.Is(err, os.ErrClosed) {
				t.Fatal("terminal status retained the file handle", err)
			}
		})
	}
}
