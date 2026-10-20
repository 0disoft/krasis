package netobserve

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestRestorationWakesOneRetryWithoutTreatingPowerAsTransportEvidence(t *testing.T) {
	var events []string
	m := newMonitor(context.Background(), func(code string) { events = append(events, code) })
	defer m.cancel()
	m.observe(Snapshot{Power: PowerAC, Link: LinkDown}, time.Now())
	m.observe(Snapshot{Power: PowerBattery, Saver: SaverOn, Link: LinkDown}, time.Now())
	if len(m.restored) != 0 {
		t.Fatal("power change claimed restored transport")
	}
	m.observe(Snapshot{Power: PowerBattery, Saver: SaverOn, Link: LinkUsable}, time.Now())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Wait(ctx, time.Hour); err != nil {
		t.Fatal("restoration did not shorten retry", err)
	}
	if len(m.restored) != 0 {
		t.Fatal("notification was not consumed once")
	}
	count := len(events)
	if err := m.Wait(ctx, time.Millisecond); err != nil || len(events) != count {
		t.Fatal("same event woke another retry", err)
	}
	if events[len(events)-1] != "network_retry_woken" {
		t.Fatal("retry outcome missing", events)
	}
}

func TestUnknownAndStaleObservationsCannotWakeRetry(t *testing.T) {
	m := newMonitor(context.Background(), nil)
	defer m.cancel()
	m.observe(Snapshot{}, time.Now())
	m.observe(Snapshot{Link: LinkUsable}, time.Now())
	if len(m.restored) != 0 {
		t.Fatal("unknown was treated as observed disconnection")
	}
	m.observe(Snapshot{Link: LinkDown}, time.Now())
	m.observe(Snapshot{Link: LinkUsable}, time.Now().Add(-time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Wait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal("restoration defeated cancellation", err)
	}
	if err := m.Wait(context.Background(), time.Millisecond); err != nil || len(m.restored) != 0 {
		t.Fatal("stale notification survived", err)
	}
}

func TestLocalPathChangeHasOneBoundedNotification(t *testing.T) {
	m := newMonitor(context.Background(), nil)
	defer m.cancel()
	s := Snapshot{Link: LinkUsable, topologyKnown: true, topology: [32]byte{1}}
	m.observe(s, time.Now())
	m.observe(s, time.Now())
	if len(m.restored) != 0 {
		t.Fatal("unchanged topology triggered retry")
	}
	for i := byte(2); i < 10; i++ {
		s.topology[0] = i
		m.observe(s, time.Now())
	}
	if len(m.restored) != 1 {
		t.Fatal("path changes grew an unbounded retry queue")
	}
}

func TestUnknownProviderValuesCannotInventObservations(t *testing.T) {
	var events []string
	m := newMonitor(context.Background(), func(code string) { events = append(events, code) })
	defer m.cancel()
	m.observe(Snapshot{Power: 255, Saver: 255, Link: 255}, time.Now())
	want := []string{"network_adapter_power_unknown", "network_roaming_unknown", "network_policy_unknown", "network_power_unknown", "network_saver_unknown", "network_link_unknown", "network_observation_unavailable"}
	if !reflect.DeepEqual(events, want) || len(m.restored) != 0 {
		t.Fatal("invalid provider values claimed an OS observation", events)
	}
}

func TestMonitorCloseJoinsSamplerAndCancelsExternalWait(t *testing.T) {
	var samples atomic.Int32
	first := make(chan struct{}, 1)
	m := start(context.Background(), func() Snapshot { samples.Add(1); first <- struct{}{}; return Snapshot{} }, nil)
	<-first
	m.Close()
	m.Close()
	if samples.Load() != 1 {
		t.Fatal("unexpected repeated sample")
	}
	if err := m.Wait(context.Background(), time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal("closed monitor granted retry", err)
	}
}

func TestInterfaceSamplingHasStablePrivateComparisonAndUnknownFailure(t *testing.T) {
	first := net.Interface{Index: 3, Name: "private-vpn-name", Flags: net.FlagUp}
	second := net.Interface{Index: 5, Flags: net.FlagUp | net.FlagLoopback}
	ip := &net.IPNet{IP: net.ParseIP("192.0.2.10"), Mask: net.CIDRMask(24, 32)}
	list := func() ([]net.Interface, error) { return []net.Interface{first, second}, nil }
	addresses := func(net.Interface) ([]net.Addr, error) { return []net.Addr{ip}, nil }
	s := sampleInterfaces(list, addresses)
	if s.Link != LinkUsable || !s.topologyKnown {
		t.Fatal("usable interface not observed")
	}
	reordered := sampleInterfaces(func() ([]net.Interface, error) { return []net.Interface{second, first}, nil }, addresses)
	if !reflect.DeepEqual(s, reordered) {
		t.Fatal("interface enumeration order became a path change")
	}
	failed := sampleInterfaces(list, func(net.Interface) ([]net.Addr, error) { return nil, errors.New("private-path") })
	if failed.Link != LinkUnknown || failed.topologyKnown {
		t.Fatal("failed inspection became disconnection")
	}
	failed = sampleInterfaces(func() ([]net.Interface, error) { return nil, errors.New("private-path") }, addresses)
	if failed.Link != LinkUnknown {
		t.Fatal("failed inventory became observed absence")
	}
	first.Flags = 0
	down := sampleInterfaces(list, addresses)
	if down.Link != LinkDown || !down.topologyKnown {
		t.Fatal("known offline interface not observed")
	}
}
