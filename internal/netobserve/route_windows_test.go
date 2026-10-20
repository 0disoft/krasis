//go:build windows

package netobserve

import (
	"net/netip"
	"testing"
)

func TestBestRouteWindowsLoopback(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "::1"} {
		if got := bestRoute(netip.MustParseAddr(s)); got == 0 {
			t.Fatalf("loopback route unavailable for %s", s)
		}
	}
	if bestRoute(netip.Addr{}) != 0 {
		t.Fatal("invalid address accepted")
	}
}
