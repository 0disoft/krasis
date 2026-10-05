package handoff

import (
	"context"
	"time"
)

type FocusStage uint8

const (
	FocusBeforeShow FocusStage = iota
	FocusBeforeRestore
	FocusAfterRestore
	FocusAfterHide
	FocusStageCount
)

// State 0 is unobserved; 1 NULL, 2 capture, 3 saved original, 4 other.
// No native handles or window/application metadata leave the owner thread.
type FocusProfile struct {
	Schema         uint8     `json:"schema"`
	States         [4]uint8  `json:"states"`
	Observed       uint8     `json:"observed"`
	Restore        uint8     `json:"restore"`
	PostHide       uint8     `json:"postHide"`
	ResultMeasured uint8     `json:"resultMeasured"`
	Micros         [2]uint32 `json:"micros"`
	Measured       uint8     `json:"measured"`
}

func (p FocusProfile) Valid() bool {
	if p.Schema != 1 || p.Observed>>FocusStageCount != 0 || p.ResultMeasured>>2 != 0 || p.Measured>>2 != 0 {
		return false
	}
	for i, state := range p.States {
		if state > 4 || (p.Observed&(1<<i) != 0) != (state != 0) {
			return false
		}
	}
	for i, result := range [2]uint8{p.Restore, p.PostHide} {
		max := uint8(4)
		if i == 1 {
			max = 3
		}
		if result > max || (p.ResultMeasured&(1<<i) != 0) != (result != 0) {
			return false
		}
	}
	for i, n := range p.Micros {
		if n > uint32(Window.Microseconds()) || (p.Measured&(1<<i) == 0 && n != 0) {
			return false
		}
	}
	return p.Observed|p.ResultMeasured|p.Measured != 0
}

// FocusEnabled gates optional native observations to the original live source
// attempt. It never authorizes focus changes or controls cleanup behavior.
func FocusEnabled(ctx context.Context) bool {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil {
		return false
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	return focusLive(s, a)
}

func focusLive(s *session, a *attempt) bool {
	return !a.finished && !s.collector.stopped.Load() && time.Since(s.collector.started) < Window && a.summary.Role == "source"
}

func updateFocus(ctx context.Context, update func(*FocusProfile)) {
	s, a := getSession(ctx), getAttempt(ctx)
	if s == nil || a == nil {
		return
	}
	s.collector.mu.Lock()
	defer s.collector.mu.Unlock()
	if !focusLive(s, a) {
		return
	}
	if a.summary.Focus == nil {
		a.summary.Focus = &FocusProfile{Schema: 1}
	}
	update(a.summary.Focus)
}

func RecordFocusState(ctx context.Context, stage FocusStage, state uint8) {
	if stage >= FocusStageCount || state < 1 || state > 4 {
		return
	}
	updateFocus(ctx, func(p *FocusProfile) {
		if p.Observed&(1<<stage) == 0 {
			p.States[stage] = state
			p.Observed |= 1 << stage
		}
	})
}

// Index 0 is restore, 1 is post-hide. Preserve the first cleanup's result.
func RecordFocusResult(ctx context.Context, index, result uint8) {
	if index > 1 || result < 1 || result > 4 || (index == 1 && result > 3) {
		return
	}
	updateFocus(ctx, func(p *FocusProfile) {
		if p.ResultMeasured&(1<<index) == 0 {
			if index == 0 {
				p.Restore = result
			} else {
				p.PostHide = result
			}
			p.ResultMeasured |= 1 << index
		}
	})
}

// Index 0 is restoreFocus, 1 is hideAfterFocusRestore. Spans may overlap
// other capture timings and must not be summed with them.
func FocusSpan(ctx context.Context, index uint8) func() {
	if index > 1 || !FocusEnabled(ctx) {
		return func() {}
	}
	started := time.Now()
	return func() {
		elapsed := time.Since(started)
		if elapsed < 0 {
			elapsed = 0
		}
		if elapsed > Window {
			elapsed = Window
		}
		updateFocus(ctx, func(p *FocusProfile) {
			if p.Measured&(1<<index) == 0 {
				p.Micros[index] = uint32(elapsed.Microseconds())
				p.Measured |= 1 << index
			}
		})
	}
}
