package handoff

import (
	"context"
	"time"
)

const socketProbeInterval = 10 * time.Millisecond
const socketProbeLimit = 64

// Positive samples prove socket data was queued locally, not that the matching
// grant arrived. Empty samples do NOT prove network delay: TLS and an overlapped
// receive can already own bytes outside this queue. Sampling gaps include probe
// scheduling; nothing here is a packet-arrival timestamp or changes authority.
type SocketQueueProfile struct {
	Samples            uint8  `json:"samples"`
	PositiveSamples    uint8  `json:"positiveSamples"`
	Failures           uint8  `json:"failures"`
	FirstQueuedMicros  uint32 `json:"firstQueuedMicros"`
	LastEmptyMicros    uint32 `json:"lastEmptyMicros"`
	MaxQueuedBytes     uint32 `json:"maxQueuedBytes"`
	MaxSampleGapMicros uint32 `json:"maxSampleGapMicros"`
	MaxQueryMicros     uint32 `json:"maxQueryMicros"`
}

func (p SocketQueueProfile) valid(elapsed uint32) bool {
	if p.Samples == 0 && p.Failures == 0 {
		return p == (SocketQueueProfile{})
	}
	return int(p.Samples)+int(p.Failures) <= socketProbeLimit && p.PositiveSamples <= p.Samples &&
		(p.Samples > p.PositiveSamples || p.LastEmptyMicros == 0) &&
		(p.PositiveSamples == 0) == (p.MaxQueuedBytes == 0) &&
		(p.PositiveSamples != 0 || p.FirstQueuedMicros == 0) &&
		p.FirstQueuedMicros <= elapsed && p.LastEmptyMicros <= elapsed &&
		p.MaxSampleGapMicros <= elapsed && p.MaxQueryMicros <= elapsed
}

type socketSample struct {
	started, finished time.Time
	bytes             uint32
	ok                bool
}

// ConfigureSocketReceiveProbe starts one joined, connection-local observer.
// The callback must be a synchronous read-only socket query, not network I/O.
// Idle sessions do not poll. At most 64 queries per retained source attempt;
// unavailable stops polling that connection. No mutex is held during a query.
func ConfigureSocketReceiveProbe(ctx context.Context, query func() (uint32, bool)) func() {
	s := getSession(ctx)
	if s == nil || query == nil {
		return func() {}
	}
	s.collector.mu.Lock()
	if s.reader.ready || s.socketWake != nil {
		s.collector.mu.Unlock()
		return func() {}
	}
	s.socketWake = make(chan struct{}, 1)
	wake := s.socketWake
	s.collector.mu.Unlock()
	probeCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-probeCtx.Done():
				return
			case <-wake:
			}
			timer := time.NewTicker(socketProbeInterval)
			active, supported := true, true
			for active && supported {
				select {
				case <-probeCtx.Done():
					active = false
				case <-timer.C:
					active, supported = sampleSocketQueue(s, query)
				}
			}
			timer.Stop()
			if !supported {
				return
			}
		}
	}()
	return func() { cancel(); <-done }
}

func sampleSocketQueue(s *session, query func() (uint32, bool)) (active, supported bool) {
	a := s.pendingGrant.Load()
	if a == nil || s.collector.stopped.Load() {
		return false, true
	}
	s.collector.mu.Lock()
	p := currentGrantProgress(s)
	eligible := p != nil && s.source == a && p.socketCount < socketProbeLimit
	s.collector.mu.Unlock()
	if !eligible {
		return false, true
	}
	started := time.Now()
	bytes, ok := query()
	value := socketSample{started: started, finished: time.Now(), bytes: bytes, ok: ok}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if currentGrantProgress(s) != p || s.source != a || p.socketCount >= socketProbeLimit {
		return false, ok
	}
	p.socketSamples[p.socketCount] = value
	p.socketCount++
	return p.socketCount < socketProbeLimit, ok
}

func (p *grantProgress) socketSnapshot(end time.Time) SocketQueueProfile {
	var result SocketQueueProfile
	previous := p.start
	for _, value := range p.socketSamples[:p.socketCount] {
		// Dispatch can finish after the grant was already read. Never attach
		// those later observations, or a late old-epoch callback, to its wait.
		if value.started.Before(p.start) || value.finished.Before(value.started) || value.finished.After(end) {
			continue
		}
		at := uint32(value.finished.Sub(p.start).Microseconds())
		result.MaxQueryMicros = max(result.MaxQueryMicros, uint32(value.finished.Sub(value.started).Microseconds()))
		result.MaxSampleGapMicros = max(result.MaxSampleGapMicros, uint32(value.finished.Sub(previous).Microseconds()))
		previous = value.finished
		if !value.ok {
			result.Failures++
			continue
		}
		result.Samples++
		if value.bytes == 0 {
			result.LastEmptyMicros = at
			continue
		}
		if result.PositiveSamples == 0 {
			result.FirstQueuedMicros = at
		}
		result.PositiveSamples++
		result.MaxQueuedBytes = max(result.MaxQueuedBytes, value.bytes)
	}
	if result.Samples+result.Failures > 0 {
		result.MaxSampleGapMicros = max(result.MaxSampleGapMicros, uint32(end.Sub(previous).Microseconds()))
	}
	return result
}
