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

func TestDisplayChangeRecoveryWaitsForStableGeometryAndRunsOnce(t *testing.T) {
	const alias = "B"
	fp := strings.Repeat("a", 64)
	now := time.Now()
	r := AutomaticRecovery{}
	r.Next(now, View{State: "CONNECTED", PeerAlias: alias, Fingerprints: map[string]string{alias: fp}})
	local := View{State: "LOCAL", StopReason: "display_change", Fingerprints: map[string]string{alias: fp}}
	if got := r.Next(now, local); got != "" {
		t.Fatal(got)
	}
	local.DisplayReady = true
	if got := r.Next(now.Add(2*time.Second), local); got != "" {
		t.Fatal("reconnected before geometry settled", got)
	}
	local.DisplayReady = false
	if got := r.Next(now.Add(4*time.Second), local); got != "" {
		t.Fatal("reconnected on unstable geometry", got)
	}
	local.DisplayReady = true
	if got := r.Next(now.Add(5*time.Second), local); got != "display_reconnect" {
		t.Fatal(got)
	}
	if got := r.Next(now.Add(10*time.Second), local); got != "" {
		t.Fatal("replayed one-shot recovery", got)
	}
	r.Stop()
	if got := r.Next(now.Add(11*time.Second), local); got != "" {
		t.Fatal("manual stop rearmed display recovery", got)
	}
}

func TestDisplayChangeRecoveryExpiresWithoutStableGeometryOrIdentity(t *testing.T) {
	fp := strings.Repeat("a", 64)
	now := time.Now()
	r := AutomaticRecovery{}
	r.Next(now, View{State: "CONNECTED", PeerAlias: "B", Fingerprints: map[string]string{"B": fp}})
	local := View{State: "LOCAL", StopReason: "display_change", Fingerprints: map[string]string{"B": strings.Repeat("b", 64)}, DisplayReady: true}
	if got := r.Next(now, local); got != "" || r.Alias != "" {
		t.Fatal("changed identity retained", got)
	}
	r.Next(now, View{State: "CONNECTED", PeerAlias: "B", Fingerprints: map[string]string{"B": fp}})
	local.Fingerprints["B"] = fp
	local.DisplayReady = false
	r.Next(now, local)
	if got := r.Next(now.Add(31*time.Second), local); got != "display_recovery_expired" {
		t.Fatal(got)
	}
	local.DisplayReady = true
	if got := r.Next(now.Add(32*time.Second), local); got != "" {
		t.Fatal("expired recovery rearmed", got)
	}
}
