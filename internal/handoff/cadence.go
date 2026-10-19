package handoff

import "time"

// CadenceProfile summarizes local input-dispatch start intervals. Intentional
// idle, preceding work, transport and scheduling can all contribute to a gap;
// it is not input-to-display latency. No action kind or input value is retained.
type CadenceProfile struct {
	Schema                 uint8     `json:"schema"`
	Intervals              uint32    `json:"intervals"`
	Histogram              [6]uint32 `json:"histogram"`
	MaxGapMicros           uint32    `json:"maxGapMicros"`
	PreviousDispatchMicros uint32    `json:"previousDispatchMicros"`
	BetweenDispatchMicros  uint32    `json:"betweenDispatchMicros"`
	Skipped                uint32    `json:"skipped,omitempty"`
	Saturated              bool      `json:"saturated,omitempty"`
}

func (p CadenceProfile) Valid() bool {
	if p.Schema != 1 || p.Intervals == 0 && p.Skipped == 0 || p.MaxGapMicros > uint32(Window.Microseconds()) {
		return false
	}
	var total uint64
	maxBucket := cadenceBucket(time.Duration(p.MaxGapMicros) * time.Microsecond)
	for i, n := range p.Histogram {
		if i > maxBucket && n != 0 {
			return false
		}
		total += uint64(n)
	}
	parts := uint64(p.PreviousDispatchMicros) + uint64(p.BetweenDispatchMicros)
	if total != uint64(p.Intervals) || parts > uint64(p.MaxGapMicros) {
		return false
	}
	if p.Intervals == 0 {
		return p.MaxGapMicros == 0 && parts == 0 && !p.Saturated
	}
	// Flooring two parts can lose one microsecond relative to the whole.
	return uint64(p.MaxGapMicros)-parts <= 1 && p.Histogram[maxBucket] != 0 &&
		(!p.Saturated || p.Intervals == ^uint32(0))
}

type inputCadence struct {
	active             *sample
	started, completed time.Time
	profile            CadenceProfile
}

// Caller owns the existing collector mutex. There is no new lock, timer,
// per-frame allocation, system call or I/O on the input path.
func (c *inputCadence) start(v *sample) {
	now := v.started
	if !c.started.IsZero() {
		gap := now.Sub(c.started)
		if c.completed.IsZero() || c.completed.Before(c.started) || now.Before(c.completed) || gap > Window {
			if c.profile.Skipped != ^uint32(0) {
				c.profile.Skipped++
			}
		} else if c.profile.Intervals == ^uint32(0) {
			c.profile.Saturated = true
		} else {
			c.profile.Intervals++
			c.profile.Histogram[cadenceBucket(gap)]++
			micros := uint32(gap.Microseconds())
			if c.profile.Intervals == 1 || micros > c.profile.MaxGapMicros {
				c.profile.MaxGapMicros = micros
				c.profile.PreviousDispatchMicros = uint32(c.completed.Sub(c.started).Microseconds())
				c.profile.BetweenDispatchMicros = uint32(now.Sub(c.completed).Microseconds())
			}
		}
	}
	c.active, c.started, c.completed = v, now, time.Time{}
}

func (c *inputCadence) complete(v *sample, completed time.Time) {
	if c.active == v && c.completed.IsZero() {
		c.completed = completed
	}
}

func (c *inputCadence) snapshot() *CadenceProfile {
	if c.profile.Intervals == 0 && c.profile.Skipped == 0 {
		return nil
	}
	p := c.profile
	p.Schema = 1
	return &p
}

// Buckets are <1ms, <4ms, <10ms, <25ms, <100ms and >=100ms.
func cadenceBucket(gap time.Duration) int {
	for i, limit := range [...]time.Duration{time.Millisecond, 4 * time.Millisecond, 10 * time.Millisecond, 25 * time.Millisecond, 100 * time.Millisecond} {
		if gap < limit {
			return i
		}
	}
	return 5
}
