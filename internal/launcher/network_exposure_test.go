package launcher

import (
	"net"
	"strconv"
	"testing"
	"time"
)

func TestBuildSessionPlanBindsListenerToConfiguredInterface(t *testing.T) {
	base := t.TempDir()
	config := DefaultConfig("hp15")
	config.PeerHost = testPeerHost
	createLauncherFixture(t, base, config.CredentialDirectory)

	plan, err := config.BuildSessionPlan(base, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 1 {
		t.Fatalf("commands = %d", len(plan.Commands))
	}
	args := plan.Commands[0].Args
	selected := net.JoinHostPort(testPeerHost, strconv.Itoa(config.Port))
	if !contains(args, selected) {
		t.Fatalf("listener args do not bind the selected interface %q: %v", selected, args)
	}
	if contains(args, net.JoinHostPort("0.0.0.0", strconv.Itoa(config.Port))) ||
		contains(args, net.JoinHostPort("::", strconv.Itoa(config.Port))) {
		t.Fatalf("listener plan contains a wildcard bind: %v", args)
	}
	if contains(args, "--server-name") {
		t.Fatalf("listener plan contains connector-only server-name: %v", args)
	}
}
