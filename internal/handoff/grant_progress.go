package handoff

import (
	"context"
	"sync/atomic"
	"time"
)

// These are reader phases, not socket or TCP states. A blocked TLS read also
// includes scheduling and peer processing; no phase proves packet loss.
type ReadPhase uint8

const (
	ReadWaiting ReadPhase = iota
	ReadDispatch
	ReadBetweenFrames
)

const maxProgressCount = uint32(1<<32 - 1)

type GrantProgressProfile struct {
	Schema               uint8     `json:"schema"`
	Micros               [3]uint32 `json:"micros"`
	ElapsedMicros        uint32    `json:"elapsedMicros"`
	ByteReads            uint32    `json:"byteReads"`
	Bytes                uint32    `json:"bytes"`
	Frames               uint32    `json:"frames"`
	MaxByteGapMicros     uint32    `json:"maxByteGapMicros"`
	ByteProgressMeasured bool      `json:"byteProgressMeasured"`
	Complete             bool      `json:"complete"`
	Saturated            bool      `json:"saturated"`
}

func (p GrantProgressProfile) Valid(role string) bool {
	if role != "source" || p.Schema != 1 || p.ElapsedMicros > uint32(Window.Microseconds()) || p.MaxByteGapMicros > p.ElapsedMicros {
		return false
	}
	total := uint64(p.Micros[0]) + uint64(p.Micros[1]) + uint64(p.Micros[2])
	if total > uint64(p.ElapsedMicros) || uint64(p.ElapsedMicros)-total > 2 {
		return false
	}
	if (p.ByteReads == 0) != (p.Bytes == 0) || (!p.Saturated && p.ByteReads > p.Bytes) {
		return false
	}
	if p.Saturated && p.ByteReads != maxProgressCount && p.Bytes != maxProgressCount && p.Frames != maxProgressCount {
		return false
	}
	return p.ByteProgressMeasured || (p.ByteReads == 0 && p.Bytes == 0 && p.MaxByteGapMicros == 0 && !p.Saturated)
}

type grantReader struct {
	ready, bytes bool
	phase        atomic.Uint32
}

type grantProgress struct {
	profile  GrantProgressProfile
	start    time.Time
	last     time.Time
	lastByte time.Time
	spans    [3]time.Duration
	phase    ReadPhase
	ended    bool
}

// StartGrantProgress runs before the session's reader goroutine. Only a source
// attempt that begins with this observer present gets a profile.
func StartGrantProgress(ctx context.Context, bytes bool, at time.Time) {
	s := getSession(ctx)
	if s == nil || s.collector.stopped.Load() || at.IsZero() {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if !s.reader.ready {
		s.reader.ready, s.reader.bytes = true, bytes
		s.reader.phase.Store(uint32(ReadBetweenFrames))
	}
}

func beginGrantProgress(s *session, a *attempt, at time.Time) {
	if a.summary.Role == "source" && s.reader.ready {
		a.progress = &grantProgress{profile: GrantProgressProfile{Schema: 1, ByteProgressMeasured: s.reader.bytes}, start: at, last: at, lastByte: at, phase: ReadPhase(s.reader.phase.Load())}
		s.pendingGrant.Store(a)
	}
}

// ReadProgress advances one phase under the collector lock, clipping at this
// attempt's start. Caller timestamps stay in memory and are never exported.
func ReadProgress(ctx context.Context, phase ReadPhase, at time.Time, frameCompleted bool) {
	s := getSession(ctx)
	if s == nil || s.collector.stopped.Load() || phase > ReadBetweenFrames || at.IsZero() {
		return
	}
	if !s.reader.ready {
		return
	}
	previous := ReadPhase(s.reader.phase.Swap(uint32(phase)))
	if s.pendingGrant.Load() == nil {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if p := currentGrantProgress(s); p != nil && p.advance(previous, at) {
		p.phase = phase
		if frameCompleted {
			p.add(&p.profile.Frames, 1)
		}
	}
}

func ReadProgressBytes(ctx context.Context, count int, at time.Time) {
	s := getSession(ctx)
	if s == nil || s.collector.stopped.Load() || s.pendingGrant.Load() == nil || count <= 0 || at.IsZero() {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	p := currentGrantProgress(s)
	if p == nil || !p.profile.ByteProgressMeasured || at.Before(p.lastByte) || at.Before(p.start) || at.Sub(p.start) > Window {
		return
	}
	p.observeGap(at)
	p.lastByte = at
	p.add(&p.profile.ByteReads, 1)
	p.add(&p.profile.Bytes, uint64(count))
}

func currentGrantProgress(s *session) *grantProgress {
	a := s.source
	if a == nil || !exchangeActive(s, a) || a.progress == nil || a.progress.ended {
		return nil
	}
	return a.progress
}

func (p *grantProgress) add(value *uint32, n uint64) {
	if n > uint64(maxProgressCount-*value) {
		*value, p.profile.Saturated = maxProgressCount, true
	} else {
		*value += uint32(n)
	}
}

func (p *grantProgress) advance(phase ReadPhase, at time.Time) bool {
	if at.Before(p.last) || at.Sub(p.start) > Window {
		return false
	}
	p.spans[phase] += at.Sub(p.last)
	p.last = at
	return true
}

func (p *grantProgress) observeGap(at time.Time) {
	if p.profile.ByteProgressMeasured && !at.Before(p.lastByte) {
		p.profile.MaxByteGapMicros = max(p.profile.MaxByteGapMicros, uint32(at.Sub(p.lastByte).Microseconds()))
	}
}

func finishGrantProgress(s *session, a *attempt, at time.Time, complete bool) {
	finishGrantProgressAt(a, ReadPhase(s.reader.phase.Load()), at, complete)
	s.pendingGrant.CompareAndSwap(a, nil)
}

func finishGrantProgressAt(a *attempt, phase ReadPhase, at time.Time, complete bool) {
	p := a.progress
	if p == nil || p.ended {
		return
	}
	if !p.advance(phase, at) {
		// Late retirement keeps only the last valid observed endpoint.
		at = p.last
		complete = false
	}
	p.observeGap(at)
	p.profile.ElapsedMicros = uint32(at.Sub(p.start).Microseconds())
	for i, elapsed := range p.spans {
		p.profile.Micros[i] = uint32(elapsed.Microseconds())
	}
	p.profile.Complete, p.ended = complete, true
	if p.profile.Valid("source") {
		profile := p.profile
		a.summary.GrantProgress = &profile
	}
}
