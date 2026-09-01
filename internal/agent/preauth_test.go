package agent

import (
	"net"
	"testing"
	"time"
)

func TestAddressRateLimiterAllowsBurstThenRefills(t *testing.T) {
	policy := DefaultPreAuthPolicy()
	policy.PerAddressBurst = 2
	policy.PerAddressRefillInterval = time.Second
	limiter := newAddressRateLimiter(policy)
	address := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 1000}
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	if !limiter.allow(address, now) || !limiter.allow(address, now) {
		t.Fatal("initial burst was rejected")
	}
	if limiter.allow(address, now) {
		t.Fatal("request beyond the burst was accepted")
	}
	if !limiter.allow(address, now.Add(policy.PerAddressRefillInterval)) {
		t.Fatal("one replenished token was rejected")
	}
	if limiter.allow(address, now.Add(policy.PerAddressRefillInterval)) {
		t.Fatal("refill created more than one token")
	}
}

func TestAddressRateLimiterSharesBudgetAcrossPorts(t *testing.T) {
	policy := DefaultPreAuthPolicy()
	policy.PerAddressBurst = 1
	limiter := newAddressRateLimiter(policy)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	first := &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 1000}
	second := &net.TCPAddr{IP: net.ParseIP("2001:db8::1"), Port: 2000}
	if !limiter.allow(first, now) {
		t.Fatal("first address use was rejected")
	}
	if limiter.allow(second, now) {
		t.Fatal("port change bypassed the per-address budget")
	}
}

func TestAddressRateLimiterBoundsTrackedAddressesAndExpiresIdleEntries(t *testing.T) {
	policy := DefaultPreAuthPolicy()
	policy.MaxTrackedAddresses = 2
	policy.AddressIdleTTL = 10 * time.Second
	limiter := newAddressRateLimiter(policy)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	for index, address := range []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"} {
		if !limiter.allow(&net.TCPAddr{IP: net.ParseIP(address)}, now.Add(time.Duration(index)*time.Second)) {
			t.Fatalf("first request for %s was rejected", address)
		}
	}
	if len(limiter.buckets) != policy.MaxTrackedAddresses {
		t.Fatalf("tracked addresses = %d, want %d", len(limiter.buckets), policy.MaxTrackedAddresses)
	}
	if _, ok := limiter.buckets["192.0.2.1"]; ok {
		t.Fatal("oldest address was not evicted")
	}

	later := now.Add(policy.AddressIdleTTL + 3*time.Second)
	if !limiter.allow(&net.TCPAddr{IP: net.ParseIP("192.0.2.4")}, later) {
		t.Fatal("request after idle expiry was rejected")
	}
	if len(limiter.buckets) != 1 {
		t.Fatalf("idle entries were retained: %d", len(limiter.buckets))
	}
}

func TestPreAuthAddressKeyDoesNotRetainPorts(t *testing.T) {
	address := &staticNetworkAddress{network: "tcp", value: "[2001:db8::2]:24876"}
	if got := preAuthAddressKey(address); got != "2001:db8::2" {
		t.Fatalf("preAuthAddressKey() = %q", got)
	}
	if got := preAuthAddressKey(&staticNetworkAddress{network: "pipe", value: "opaque"}); got != "<unknown>" {
		t.Fatalf("opaque address key = %q", got)
	}
}

type staticNetworkAddress struct {
	network string
	value   string
}

func (address *staticNetworkAddress) Network() string { return address.network }
func (address *staticNetworkAddress) String() string  { return address.value }
