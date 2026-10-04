package agent

import (
	"time"

	"github.com/0disoft/krasis/internal/control"
)

const (
	preparationPreviewLead = 150 * time.Millisecond
	previewSampleMinimum   = 4 * time.Millisecond
	previewSampleAdvance   = 16 * time.Millisecond
	previewSampleMaximum   = 50 * time.Millisecond
)

// Physical pointer positions determine speed. Raw mouse deltas are used only
// for direction because acceleration and DPI can make counts differ from pixels.
// All state remains local to the serialized observer; nothing is logged or sent
// except the existing timestamp hint. This heuristic cannot predict a warp.
func (p *preparationBoundary) preview(action control.InputAction, x int32, now time.Time) bool {
	if action.Kind != control.InputPointerMove || now.IsZero() {
		return false
	}
	left, width := int64(p.config.ScreenLeft), int64(p.config.ScreenWidth)
	if width <= 1 || int64(x) < left || int64(x) >= left+width {
		p.sampleAt = time.Time{}
		return false
	}
	distance := int64(x) - left
	outward := action.DeltaX < 0
	if p.config.Edge == control.ScreenEdgeRight {
		distance = left + width - 1 - int64(x)
		outward = action.DeltaX > 0
	}
	band := p.previewBand(x, now)
	if p.sent && distance > p.rearmAt {
		p.sent = false
	}
	if p.sent || !outward || distance > band {
		return false
	}
	p.sent = true
	// Keep the threshold chosen at emission, so a falling speed estimate
	// cannot rearm the same outward approach. Actual retreat is required.
	p.rearmAt = band + int64(p.config.RearmDistance)
	p.rearmAt = min(p.rearmAt, width-2)
	return true
}

func (p *preparationBoundary) previewBand(x int32, now time.Time) int64 {
	base := min(int64(192), int64(p.config.ScreenWidth)/4)
	change := int64(x) - int64(p.sampleX)
	if p.config.Edge == control.ScreenEdgeLeft {
		change = -change
	}
	elapsed := now.Sub(p.sampleAt)
	if p.sampleAt.IsZero() || elapsed < 0 || elapsed > previewSampleMaximum || change < 0 {
		p.sampleX, p.sampleAt, p.band = x, now, base
		return base
	}
	if elapsed >= previewSampleMinimum {
		// The short observation window smooths duplicate/high-frequency
		// reports; the region cap bounds preparation on abrupt movements.
		p.band = max(base, min(change*int64(preparationPreviewLead)/int64(elapsed), int64(p.config.ScreenWidth)*3/4))
	}
	if elapsed >= previewSampleAdvance {
		p.sampleX, p.sampleAt = x, now
	}
	return p.band
}
