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

func TestApplyFailureIsBoundToPeerAndTransfer(t *testing.T) {
	m := testManager(t)
	peer, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	id := strings.Repeat("c", 32)
	m.status = Status{State: "APPLY_FAILED", Peer: peer, ID: id}
	info := m.info(peer)
	if info.UpdateID != id || info.UpdateState != "APPLY_FAILED" || m.info(other).UpdateID != "" {
		t.Fatal("apply result leaked or omitted")
	}
	data, _ := json.Marshal(info)
	if _, err := readPeerInfo(strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"DELIVERED", "TRANSFER_UNKNOWN", "RESTART_UNCONFIRMED"} {
		m.status = Status{State: state, Peer: peer, ID: id}
		m.observePeer(other, info)
		stale := info
		stale.UpdateID = strings.Repeat("d", 32)
		m.observePeer(peer, stale)
		if m.status.State != state {
			t.Fatal("unrelated failure accepted")
		}
		m.observePeer(peer, info)
		if m.status.State != "PEER_APPLY_FAILED" {
			t.Fatal("matching failure lost")
		}
	}
}

func TestApplyResultValidationAndLateConfirmation(t *testing.T) {
	for _, suffix := range []string{`,"updateState":"private error"`, `,"updateState":"APPLY_FAILED","updateID":"bad"`, `,"updateID":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`} {
		if _, err := readPeerInfo(strings.NewReader(`{"protocol":2,"generation":27` + suffix + `}`)); err == nil {
			t.Fatal("invalid result accepted")
		}
	}
	for _, state := range []string{"TRANSFER_UNKNOWN", "RESTART_UNCONFIRMED"} {
		m := testManager(t)
		peer, rev := strings.Repeat("a", 64), strings.Repeat("b", 40)
		m.status = Status{State: state, Peer: peer, Generation: 28, Revision: rev}
		m.observePeer(peer, PeerInfo{Generation: 28, Revision: rev})
		if m.status.State != "UPDATED" {
			t.Fatal("late runtime confirmation lost")
		}
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
