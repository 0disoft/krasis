package peerupdate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPeerPreflightOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		generation int
		revision   string
		allowed    bool
		want       string
	}{
		{"same", 24, strings.Repeat("c", 40), true, "SAME_VERSION"},
		{"newer", 25, strings.Repeat("d", 40), true, "PEER_NEWER"},
		{"different", 24, strings.Repeat("d", 40), true, "GENERATION_CONFLICT"},
		{"denied", 23, "", false, "PEER_NOT_ALLOWED"},
		{"older", 23, "", true, "QUEUED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := testManager(t)
			peer := strings.Repeat("a", 64)
			m.out = &offer{peer: peer}
			m.status = Status{State: "QUEUED", Peer: peer, Generation: 24, Revision: strings.Repeat("c", 40)}
			m.observePeer(peer, PeerInfo{Protocol: 2, Generation: tc.generation, Revision: tc.revision, Allowed: tc.allowed})
			if m.status.State != tc.want {
				t.Fatalf("got %s", m.status.State)
			}
			if tc.want != "QUEUED" && m.out != nil {
				t.Fatal("rejected offer retained")
			}
		})
	}
}

func TestInstalledConfirmationRequiresExactPeerAndBuild(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	rev := strings.Repeat("c", 40)
	m.status = Status{State: "DELIVERED", Peer: peer, Generation: 24, Revision: rev}
	for _, tc := range []struct {
		peer string
		gen  int
		rev  string
	}{{strings.Repeat("b", 64), 24, rev}, {peer, 23, rev}, {peer, 24, strings.Repeat("d", 40)}} {
		m.observePeer(tc.peer, PeerInfo{Generation: tc.gen, Revision: tc.rev})
		if m.status.State != "DELIVERED" {
			t.Fatal("unrelated build confirmed")
		}
	}
	m.observePeer(peer, PeerInfo{Generation: 24, Revision: rev})
	if m.status.State != "UPDATED" {
		t.Fatal("exact runtime not confirmed")
	}
}

func TestStatusEndpointAuthorizationAndBounds(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	data, _ := json.Marshal(PeerInfo{Protocol: 2, Generation: 24, Revision: strings.Repeat("b", 40)})
	for _, tc := range []struct {
		allowed bool
		body    string
		code    int
	}{{false, string(data), 403}, {true, string(data), 200}, {true, strings.Repeat("x", 2049), 400}, {true, `{"protocol":2,"generation":24,"revision":"private text"}`, 400}, {true, `{"protocol":3,"generation":24}`, 400}} {
		req := httptest.NewRequest(http.MethodPost, "/v2/status", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		m.handler(peer, func() bool { return tc.allowed }).ServeHTTP(w, req)
		if w.Code != tc.code {
			t.Fatalf("got %d want %d", w.Code, tc.code)
		}
	}
}

func TestMissingPeerAndRestartTimeoutAreNotSuccess(t *testing.T) {
	m := testManager(t)
	peer := strings.Repeat("a", 64)
	m.out = &offer{peer: peer, created: time.Now().Add(-20 * time.Second), expires: time.Now().Add(time.Minute)}
	m.status = Status{State: "QUEUED", Peer: peer}
	if m.Status().State != "PEER_UNREACHABLE" {
		t.Fatal("missing peer remained queued")
	}
	m.status = Status{State: "DELIVERED", Peer: peer}
	m.deliveredUntil = time.Now().Add(-time.Second)
	if m.Status().State != "RESTART_UNCONFIRMED" {
		t.Fatal("receipt treated as installation")
	}
}
