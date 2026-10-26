package agent

import (
	"time"

	"github.com/0disoft/krasis/internal/control"
)

const (
	// Lead plus the bounded entry wait can cover the optional startup budget.
	// The region cap and launch cooldown still bound unused preparation.
	preparationPreviewLead = 500 * time.Millisecond
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
		p.resumeUntil = time.Time{}
		return false
	}
	if !p.movementAt.IsZero() {
		gap := now.Sub(p.movementAt)
		if gap < 0 {
			return false
		}
		if gap >= preparationHintRenewal {
			// Resume work on real movement, before a fresh velocity sample or
			// entry into the edge band. Retain a short admission retry window
			// if the nonblocking hook queue is full. Idle itself launches nothing.
			p.resumeUntil = now.Add(preparationHintLifetime)
		}
	}
	p.movementAt = now
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
	if p.sent && (p.sentAt.IsZero() || now.Sub(p.sentAt) < preparationHintRenewal) {
		return false
	}
	resuming := !p.resumeUntil.IsZero() && now.Before(p.resumeUntil)
	// A completed retreat starts a new approach. If the last hint is already
	// outside the sender/receiver launch cooldown, use its first outward motion
	// even during continuous use. Waiting for another velocity sample/edge band
	// can put child construction back on the approved entry's critical path.
	renewing := !p.sent && !p.sentAt.IsZero() && now.Sub(p.sentAt) >= preparationHintRenewal
	if !outward || distance > band && !resuming && !renewing {
		return false
	}
	if resuming || renewing {
		// Suppress another hint on the same approach even when this first
		// movement was outside the normal prediction band.
		p.band = max(band, distance)
	}
	return true
}

// Commit approach suppression only after the bounded queue accepts the hint.
func (p *preparationBoundary) preparationHintSent(now time.Time) {
	p.sent = true
	p.sentAt = now
	p.resumeUntil = time.Time{}
	// Keep the threshold chosen at emission, so a falling speed estimate
	// cannot rearm the same outward approach. Retreat or a bounded expiry
	// followed by fresh outward movement is required.
	p.rearmAt = p.band + int64(p.config.RearmDistance)
	p.rearmAt = min(p.rearmAt, int64(p.config.ScreenWidth)-2)
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
