package agent

import (
	"net"
	"testing"
	"time"
)

func TestPreAuthAdmissionLimitsOneSourceWithoutPersistingAddresses(t *testing.T) {
	config := PreAuthAdmissionConfig{
		GlobalTokensPerSecond:    100,
		GlobalBurst:              100,
		PerSourceTokensPerSecond: 1,
		PerSourceBurst:           2,
		MaxTrackedSources:        4,
		SourceIdleTTL:            time.Minute,
	}
	admission, err := newPreAuthAdmission(config)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	source := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 5000}
	if decision := admission.allow(source, now); decision != preAuthAdmitted {
		t.Fatalf("first decision = %v", decision)
	}
	if decision := admission.allow(source, now); decision != preAuthAdmitted {
		t.Fatalf("second decision = %v", decision)
	}
	if decision := admission.allow(source, now); decision != preAuthRejectedSourceRate {
		t.Fatalf("third decision = %v", decision)
	}
	if decision := admission.allow(source, now.Add(time.Second)); decision != preAuthAdmitted {
		t.Fatalf("refilled decision = %v", decision)
	}
}

func TestPreAuthAdmissionGlobalLimitSurvivesAddressRotation(t *testing.T) {
	config := PreAuthAdmissionConfig{
		GlobalTokensPerSecond:    1,
		GlobalBurst:              2,
		PerSourceTokensPerSecond: 100,
		PerSourceBurst:           100,
		MaxTrackedSources:        8,
		SourceIdleTTL:            time.Minute,
	}
	admission, err := newPreAuthAdmission(config)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	for index := 1; index <= 2; index++ {
		source := &net.TCPAddr{IP: net.IPv4(198, 51, 100, byte(index)), Port: 5000}
		if decision := admission.allow(source, now); decision != preAuthAdmitted {
			t.Fatalf("decision %d = %v", index, decision)
		}
	}
	rotated := &net.TCPAddr{IP: net.ParseIP("198.51.100.3"), Port: 5000}
	if decision := admission.allow(rotated, now); decision != preAuthRejectedGlobalRate {
		t.Fatalf("rotated decision = %v", decision)
	}
}

func TestPreAuthAdmissionBoundsAndExpiresSourceState(t *testing.T) {
	config := PreAuthAdmissionConfig{
		GlobalTokensPerSecond:    100,
		GlobalBurst:              100,
		PerSourceTokensPerSecond: 100,
		PerSourceBurst:           100,
		MaxTrackedSources:        2,
		SourceIdleTTL:            time.Second,
	}
	admission, err := newPreAuthAdmission(config)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	first := &net.TCPAddr{IP: net.ParseIP("203.0.113.1"), Port: 5000}
	second := &net.TCPAddr{IP: net.ParseIP("203.0.113.2"), Port: 5000}
	third := &net.TCPAddr{IP: net.ParseIP("203.0.113.3"), Port: 5000}
	if decision := admission.allow(first, now); decision != preAuthAdmitted {
		t.Fatal(decision)
	}
	if decision := admission.allow(second, now); decision != preAuthAdmitted {
		t.Fatal(decision)
	}
	if decision := admission.allow(third, now); decision != preAuthRejectedSourceCapacity {
		t.Fatalf("full-table decision = %v", decision)
	}
	if decision := admission.allow(third, now.Add(time.Second)); decision != preAuthAdmitted {
		t.Fatalf("expired-table decision = %v", decision)
	}
	if len(admission.sources) != 1 {
		t.Fatalf("tracked sources = %d", len(admission.sources))
	}
}

func TestPreAuthAdmissionNormalizesIPv4MappedAddresses(t *testing.T) {
	config := PreAuthAdmissionConfig{
		GlobalTokensPerSecond:    100,
		GlobalBurst:              100,
		PerSourceTokensPerSecond: 1,
		PerSourceBurst:           1,
		MaxTrackedSources:        4,
		SourceIdleTTL:            time.Minute,
	}
	admission, err := newPreAuthAdmission(config)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	ipv4 := &net.TCPAddr{IP: net.ParseIP("192.0.2.20"), Port: 5000}
	mapped := &net.TCPAddr{IP: net.ParseIP("::ffff:192.0.2.20"), Port: 5001}
	if decision := admission.allow(ipv4, now); decision != preAuthAdmitted {
		t.Fatal(decision)
	}
	if decision := admission.allow(mapped, now); decision != preAuthRejectedSourceRate {
		t.Fatalf("mapped decision = %v", decision)
	}
}

func TestPreAuthAdmissionRejectsInvalidConfigurationAndClockRollback(t *testing.T) {
	if _, err := newPreAuthAdmission(PreAuthAdmissionConfig{}); err == nil {
		t.Fatal("expected invalid configuration error")
	}
	admission, err := newPreAuthAdmission(DefaultPreAuthAdmissionConfig())
	if err != nil {
		t.Fatal(err)
	}
	source := &net.TCPAddr{IP: net.ParseIP("192.0.2.30"), Port: 5000}
	now := time.Unix(1_700_000_000, 0)
	if decision := admission.allow(source, now); decision != preAuthAdmitted {
		t.Fatal(decision)
	}
	if decision := admission.allow(source, now.Add(-time.Second)); decision != preAuthRejectedSourceRate {
		t.Fatalf("clock rollback decision = %v", decision)
	}
}

func TestPreAuthMetricSnapshotContainsOnlyAggregateCounters(t *testing.T) {
	var metrics preAuthMetricCounters
	metrics.recordDecision(preAuthAdmitted)
	metrics.recordDecision(preAuthRejectedSourceRate)
	metrics.rejectedBusy.Add(1)
	metrics.handshakeFailed.Add(1)
	metrics.authenticated.Add(1)

	snapshot := metrics.snapshot()
	if snapshot.Admitted != 1 ||
		snapshot.RejectedBusy != 1 ||
		snapshot.RejectedSourceRate != 1 ||
		snapshot.HandshakeFailed != 1 ||
		snapshot.Authenticated != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
