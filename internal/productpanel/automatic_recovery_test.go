package productpanel

import (
	"strings"
	"testing"
	"time"
)

func TestAutomaticRecoveryBoundedAcrossBriefReconnects(t *testing.T) {
	r := AutomaticRecovery{}
	now := time.Now()
	connected := View{State: "CONNECTED", PeerAlias: "B", Fingerprints: map[string]string{"B": strings.Repeat("a", 64)}}
	for i := 0; i < 4; i++ {
		r.Next(now, connected)
		if got := r.Next(now, View{State: "FAULTED"}); got != "auto_diagnose" {
			t.Fatal(got)
		}
		if got := r.Next(now, View{State: "FAULTED"}); got != "" {
			t.Fatal("no backoff", got)
		}
		now = now.Add(time.Minute)
		got := r.Next(now, View{State: "FAULTED"})
		if i < 3 && got != "auto_recover" || i == 3 && got != "" {
			t.Fatalf("attempt %d: %q", i, got)
		}
	}
	if r.Attempts != 3 {
		t.Fatal(r.Attempts)
	}
}

func TestAutomaticRecoveryNeverStartsUnknownOrUserStoppedPeer(t *testing.T) {
	r := AutomaticRecovery{}
	now := time.Now()
	if r.Next(now, View{State: "FAULTED"}) != "auto_diagnose" {
		t.Fatal("unknown peer must still save local diagnostics")
	}
	if r.Next(now.Add(time.Minute), View{State: "FAULTED"}) != "" {
		t.Fatal("unknown peer restarted")
	}
	r.Next(now, View{State: "CONNECTED", PeerAlias: "B", Fingerprints: map[string]string{"B": strings.Repeat("a", 64)}})
	if r.Next(now, View{State: "RECONNECTING"}) != "auto_diagnose" {
		t.Fatal("missing diagnostic")
	}
	if r.Next(now.Add(time.Minute), View{State: "RECONNECTING"}) != "" {
		t.Fatal("replaced reconnecting supervisor")
	}
	r.Stop()
	r.Next(now, View{State: "CONNECTED", PeerAlias: "B", Fingerprints: map[string]string{"B": strings.Repeat("a", 64)}})
	if r.Next(now.Add(time.Minute), View{State: "FAULTED"}) != "" {
		t.Fatal("manual stop rearmed")
	}
}
