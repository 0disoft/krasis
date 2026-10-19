package handoff

import (
	"context"
	"time"
)

// TCPInfoProfile compares two reads of one source socket, before the request
// and after its matching complete grant. These are TCP estimates/counters,
// not application RTT or proof that the handoff itself lost a packet.
type TCPInfoProfile struct {
	Schema             uint8     `json:"schema"`
	RTTMicros          [2]uint32 `json:"rttMicros"`
	MinRTTMicros       [2]uint32 `json:"minRttMicros"`
	QueryMicros        [2]uint32 `json:"queryMicros"`
	RetransmittedBytes uint32    `json:"retransmittedBytes"`
	TimeoutEpisodes    uint32    `json:"timeoutEpisodes"`
	CounterDeltasKnown bool      `json:"counterDeltasKnown"`
}

func (p TCPInfoProfile) Valid(role string) bool {
	if p.Schema != 1 || role != "source" || !p.CounterDeltasKnown && (p.RetransmittedBytes != 0 || p.TimeoutEpisodes != 0) {
		return false
	}
	for i := range p.RTTMicros {
		if p.RTTMicros[i] == 0 || p.MinRTTMicros[i] == 0 || p.RTTMicros[i] > uint32(Window.Microseconds()) ||
			p.MinRTTMicros[i] > uint32(Window.Microseconds()) || p.QueryMicros[i] > uint32(Window.Microseconds()) {
			return false
		}
	}
	return true
}

type tcpInfoAttempt struct {
	before       [4]uint32
	queryMicros  uint32
	beforeKnown  bool
	endRequested bool
}

// ConfigureTCPInfo is connection-local and configured before session workers.
// The callback must be a read-only local socket query, with no traffic, retries
// or authority effects. At most two queries per retained source attempt run
// outside the collector lock and never on physical-input callback threads.
func ConfigureTCPInfo(ctx context.Context, query func() ([4]uint32, bool)) {
	s := getSession(ctx)
	if s == nil {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if s.source == nil && s.receiver == nil && s.tcpInfo == nil {
		s.tcpInfo = query
	}
}

func sampleTCPInfo(ctx context.Context, end bool) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil {
		return
	}
	c := s.collector
	c.mu.Lock()
	if !exchangeActive(s, a) || a.summary.Role != "source" || s.tcpInfo == nil ||
		end && (!a.tcpInfo.beforeKnown || a.tcpInfo.endRequested || a.grantRead.IsZero()) {
		c.mu.Unlock()
		return
	}
	if end {
		a.tcpInfo.endRequested = true
	}
	query := s.tcpInfo
	c.mu.Unlock()
	started := time.Now()
	value, ok := query()
	elapsed := uint32(min(time.Since(started), Window).Microseconds())
	if !ok || value[0] == 0 || value[1] == 0 || value[0] > uint32(Window.Microseconds()) || value[1] > uint32(Window.Microseconds()) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !exchangeActive(s, a) {
		return
	}
	if !end {
		a.tcpInfo.before, a.tcpInfo.queryMicros, a.tcpInfo.beforeKnown = value, elapsed, true
		return
	}
	b := a.tcpInfo.before
	p := &TCPInfoProfile{Schema: 1, RTTMicros: [2]uint32{b[0], value[0]}, MinRTTMicros: [2]uint32{b[1], value[1]},
		QueryMicros: [2]uint32{a.tcpInfo.queryMicros, elapsed}}
	// A counter can wrap. Never turn a decrease into a huge delta or false zero.
	if value[2] >= b[2] && value[3] >= b[3] {
		p.CounterDeltasKnown = true
		p.RetransmittedBytes, p.TimeoutEpisodes = value[2]-b[2], value[3]-b[3]
	}
	a.summary.TCPInfo = p
}
