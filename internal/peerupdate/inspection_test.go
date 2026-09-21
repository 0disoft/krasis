package peerupdate

import (
	"strings"
	"testing"
	"time"
)

func TestPeerInspectionIsFreshScopedAndReadOnly(t *testing.T) {
	peer, other := strings.Repeat("a", 64), strings.Repeat("b", 64)
	now := time.Now()
	m := &Manager{peers: map[string]peerObservation{peer: {info: PeerInfo{Protocol: 2, Generation: 32, Revision: strings.Repeat("c", 40), Allowed: true}, at: now}}, status: Status{State: "READY", Peer: other}}
	for _, tc := range []struct {
		offset time.Duration
		state  string
	}{{0, "ONLINE"}, {15 * time.Second, "ONLINE"}, {16 * time.Second, "STALE"}, {-time.Second, "STALE"}} {
		v := m.InspectPeer(peer, now.Add(tc.offset))
		if v.State != tc.state {
			t.Fatalf("unexpected freshness: %+v", v)
		}
		if tc.state != "ONLINE" && (v.Generation != 0 || v.Revision != "" || v.Allowed) {
			t.Fatal("stale metadata exposed as current")
		}
	}
	if m.InspectPeer(other, now).State != "UNKNOWN" || m.status.State != "READY" || m.status.Peer != other {
		t.Fatal("inspection changed update state or crossed peers")
	}
}
