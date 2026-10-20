// Package netobserve publishes local OS observations, never network authority
// or a diagnosis of why a transport failed. No names or addresses leave it.
package netobserve

import (
	"context"
	"crypto/rand"
	"sync"
	"time"
)

const interval = 2 * time.Second

type Power uint8

const (
	PowerUnknown Power = iota
	PowerAC
	PowerBattery
)

type Saver uint8

const (
	SaverUnknown Saver = iota
	SaverOff
	SaverOn
)

type Link uint8

const (
	LinkUnknown Link = iota
	LinkDown
	LinkUsable
)

type Snapshot struct {
	Power         Power
	Saver         Saver
	Link          Link
	topology      [32]byte // Process-local comparison only; never exported or persisted.
	topologyKnown bool
	wifi          wifiSnapshot
}

// Codes is the plain event vocabulary. Typed WiFi records have their own
// validated logger payload. Unsupported observations are
// explicitly unknown, not evidence of absence or of a particular failure cause.
func Codes() []string {
	return []string{
		"network_power_unknown", "network_power_ac", "network_power_battery",
		"network_saver_unknown", "network_saver_off", "network_saver_on",
		"network_link_unknown", "network_link_down", "network_link_usable",
		"network_adapter_power_unknown", "network_roaming_unknown", "network_policy_unknown",
		"network_local_path_changed", "network_observation_unavailable", "network_retry_woken",
		"network_wifi_unknown", "network_wifi_absent", "network_wifi_disconnected", "network_wifi_connected", "network_wifi_connecting",
		"network_roaming_available", "network_roaming_access_unavailable", "network_wifi_ap_changed", "network_wifi_network_changed",
	}
}

type Monitor struct {
	ctx                   context.Context
	cancel                context.CancelFunc
	done                  chan struct{}
	once                  sync.Once
	restored              chan time.Time
	emit                  func(string)
	previous              Snapshot
	started               bool
	lastLink              Link
	topology              [32]byte
	topologyKnown         bool
	lastWiFi              wifiState
	wifiReport            func(WiFiObservation)
	wifiKey               [32]byte
	wifiKeyKnown          bool
	wifiLastAt, wifiGapAt time.Time
}

func Start(ctx context.Context, emit func(string), report ...func(WiFiObservation)) *Monitor {
	return start(ctx, func() Snapshot { return platformProbe.read(ctx, Sample) }, emit, report...)
}
func start(ctx context.Context, sample func() Snapshot, emit func(string), report ...func(WiFiObservation)) *Monitor {
	m := newMonitor(ctx, emit)
	if len(report) > 0 {
		m.wifiReport = report[0]
	}
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if m.ctx.Err() != nil {
				return
			}
			m.observe(sample(), time.Now())
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return m
}
func newMonitor(ctx context.Context, emit func(string)) *Monitor {
	owned, cancel := context.WithCancel(ctx)
	m := &Monitor{ctx: owned, cancel: cancel, done: make(chan struct{}), restored: make(chan time.Time, 1), emit: emit}
	_, err := rand.Read(m.wifiKey[:])
	m.wifiKeyKnown = err == nil
	return m
}
func (m *Monitor) Close() { m.once.Do(func() { m.cancel(); <-m.done }) }
func (m *Monitor) event(code string) {
	if m.emit != nil {
		m.emit(code)
	}
}

// Only the monitor goroutine owns the comparison state. Notifications can
// shorten one serial supervisor wait; they never cancel a session or dial.
func (m *Monitor) observe(s Snapshot, at time.Time) {
	if s.wifi.state > wifiConnecting {
		s.wifi = wifiSnapshot{}
	}
	if s.Power > PowerBattery {
		s.Power = PowerUnknown
	}
	if s.Saver > SaverOn {
		s.Saver = SaverUnknown
	}
	if s.Link > LinkUsable {
		s.Link = LinkUnknown
	}
	if !m.started {
		m.event("network_adapter_power_unknown")
		m.event("network_policy_unknown")
	}
	if !m.started || m.previous.Power != s.Power {
		m.event([]string{"network_power_unknown", "network_power_ac", "network_power_battery"}[s.Power])
	}
	if !m.started || m.previous.Saver != s.Saver {
		m.event([]string{"network_saver_unknown", "network_saver_off", "network_saver_on"}[s.Saver])
	}
	if !m.started || m.previous.Link != s.Link {
		m.event([]string{"network_link_unknown", "network_link_down", "network_link_usable"}[s.Link])
	}
	if !s.topologyKnown && (!m.started || m.previous.topologyKnown) {
		m.event("network_observation_unavailable")
	}
	changed := s.topologyKnown && m.topologyKnown && s.topology != m.topology
	if changed {
		m.event("network_local_path_changed")
	}
	wifiRestored := m.observeWiFi(s.wifi, at)
	if wifiRestored || s.Link == LinkUsable && (m.lastLink == LinkDown || changed) {
		// Keep the newest notification if a prior active-session change was
		// never consumed. Queue capacity is always one.
		select {
		case <-m.restored:
		default:
		}
		select {
		case m.restored <- at:
		default:
		}
	}
	if s.Link != LinkUnknown {
		m.lastLink = s.Link
	}
	if s.topologyKnown {
		m.topology, m.topologyKnown = s.topology, true
	}
	m.previous, m.started = s, true
}

// Wait preserves the supervisor's single acquisition owner and LOCAL recovery
// boundary. Old observations do not speed up an unrelated future retry.
func (m *Monitor) Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.ctx.Err(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-m.ctx.Done():
			return m.ctx.Err()
		case <-timer.C:
			if err := ctx.Err(); err != nil {
				return err
			}
			return m.ctx.Err()
		case at := <-m.restored:
			if age := time.Since(at); age >= 0 && age <= interval {
				if err := ctx.Err(); err != nil {
					return err
				}
				if err := m.ctx.Err(); err != nil {
					return err
				}
				m.event("network_retry_woken")
				return nil
			}
		}
	}
}
