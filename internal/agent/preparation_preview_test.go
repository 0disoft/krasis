package agent

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func previewFixture(edge control.ScreenEdge) *preparationBoundary {
	return &preparationBoundary{config: control.BoundaryConfig{
		Edge: edge, ScreenLeft: -100, ScreenWidth: 1000, RearmDistance: 32,
	}}
}

func TestPreparationPredictsFastPhysicalApproachBeforeFixedBand(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeRight, control.ScreenEdgeLeft} {
		p := previewFixture(edge)
		start, approach, delta := int32(100), int32(250), int32(1)
		if edge == control.ScreenEdgeLeft {
			start, approach, delta = 700, 550, -1
		}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: delta}
		now := time.Unix(100, 0)
		if p.preview(move, start, now) {
			t.Fatal("unknown initial speed caused distant preparation")
		}
		if !p.preview(move, approach, now.Add(20*time.Millisecond)) {
			t.Fatal("fast approach waited for the old 192px band")
		}
		if p.band != 750 || p.rearmAt != 782 {
			t.Fatal("prediction or retreat region was not bounded")
		}
	}
}

func TestPreparationUsesPixelsRatherThanRawCountsAndRejectsSlowApproach(t *testing.T) {
	p := previewFixture(control.ScreenEdgeRight)
	now := time.Unix(100, 0)
	// A large raw delta without equivalent physical motion must not make
	// the preview region large: mouse acceleration is not assumed.
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 32767}
	p.preview(move, 100, now)
	if p.preview(move, 102, now.Add(20*time.Millisecond)) || p.band != 192 {
		t.Fatal("raw counts were interpreted as physical velocity")
	}
	if !p.preview(move, 850, now.Add(time.Second)) {
		t.Fatal("cold near-edge fallback was lost")
	}
}

func TestPreparationLeavesMoreLeadForModeratePhysicalApproach(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeRight, control.ScreenEdgeLeft} {
		p := previewFixture(edge)
		start, next, delta := int32(480), int32(520), int32(1)
		if edge == control.ScreenEdgeLeft {
			start, next, delta = 320, 280, -1
		}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: delta}
		at := time.Unix(100, 0)
		if p.preview(move, start, at) {
			t.Fatal("initial sample prepared before speed was known")
		}
		// 2px/ms predicts 500px with the larger lead. This position remains
		// 379/380px from the boundary and outside the old 300px prediction.
		if !p.preview(move, next, at.Add(20*time.Millisecond)) || p.band != 500 {
			t.Fatal("moderate approach retained the last-moment hint", p.band)
		}
	}
}

func TestPreparationPredictionDropsStaleReversedAndOutOfDisplaySamples(t *testing.T) {
	now := time.Unix(100, 0)
	for _, gap := range []time.Duration{-time.Millisecond, previewSampleMaximum + time.Millisecond} {
		p := previewFixture(control.ScreenEdgeRight)
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		p.preview(move, 100, now)
		if p.preview(move, 250, now.Add(gap)) || p.band != 192 {
			t.Fatal("stale/reversed clock produced predictive preparation")
		}
	}
	p := previewFixture(control.ScreenEdgeRight)
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	p.preview(move, 100, now)
	if p.preview(move, 50, now.Add(20*time.Millisecond)) || p.band != 192 {
		t.Fatal("physical reversal retained outward speed")
	}
	for _, x := range []int32{-101, 900} {
		if p.preview(move, x, now.Add(25*time.Millisecond)) || !p.sampleAt.IsZero() {
			t.Fatal("outside-display position was predicted")
		}
	}
}

func TestPreparationDoesNotRepeatWhenPredictionFalls(t *testing.T) {
	p := previewFixture(control.ScreenEdgeRight)
	now := time.Unix(100, 0)
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	p.preview(move, 100, now)
	if !p.preview(move, 250, now.Add(20*time.Millisecond)) {
		t.Fatal("initial predictive preparation missing")
	}
	if p.preview(move, 700, now.Add(time.Second)) || p.rearmAt != 782 {
		t.Fatal("decayed speed estimate rearmed an outward approach")
	}
	away := control.InputAction{Kind: control.InputPointerMove, DeltaX: -1}
	if p.preview(away, 0, now.Add(2*time.Second)) || p.sent {
		t.Fatal("real retreat did not rearm preparation")
	}
	if !p.preview(move, 850, now.Add(3*time.Second)) {
		t.Fatal("next approach did not prepare")
	}
}

func TestPreparationRetreatRemainsReachableOnNarrowScreens(t *testing.T) {
	now := time.Unix(100, 0)
	for _, width := range []int32{2, 8, 100, 1000} {
		p := &preparationBoundary{config: control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: width, RearmDistance: width - 1}}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		if !p.preview(move, width-1, now) {
			t.Fatal("near-edge preparation missing")
		}
		if p.preview(control.InputAction{Kind: control.InputPointerMove, DeltaX: -1}, 0, now.Add(time.Second)) || p.sent {
			t.Fatal("valid retreat could never rearm preparation")
		}
	}
}

func TestPreparationPredictionSmoothsVeryShortDuplicateSamples(t *testing.T) {
	p := previewFixture(control.ScreenEdgeRight)
	now := time.Unix(100, 0)
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	p.preview(move, 100, now)
	if p.preview(move, 250, now.Add(time.Microsecond)) || p.band != 192 {
		t.Fatal("sub-millisecond sample caused an unbounded speed estimate")
	}
	if !p.preview(move, 251, now.Add(4*time.Millisecond)) {
		t.Fatal("short samples prevented subsequent valid prediction")
	}
}

func TestPreparationRenewsAnExpiredApproachWithoutPhysicalRetreat(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeRight, control.ScreenEdgeLeft} {
		p := previewFixture(edge)
		at := time.Unix(100, 0)
		x, delta := int32(850), int32(1)
		if edge == control.ScreenEdgeLeft {
			x, delta = -50, -1
		}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: delta}
		if !p.preview(move, x, at) {
			t.Fatal("initial approach did not prepare")
		}
		if p.preview(move, x, at.Add(preparationHintRenewal-time.Nanosecond)) {
			t.Fatal("renewal ignored the cooldown margin")
		}
		if !p.preview(move, x, at.Add(preparationHintRenewal)) {
			t.Fatal("expired hint still required a distant retreat")
		}
		if p.preview(move, x, at.Add(preparationHintRenewal+time.Millisecond)) {
			t.Fatal("renewal repeated on consecutive input")
		}
		if !p.preview(move, x, at.Add(10*time.Second)) {
			t.Fatal("idle reentry did not prepare again")
		}
	}
}

func TestPreparationExpiryDoesNotMakeIneligibleInputRenew(t *testing.T) {
	at := time.Unix(100, 0)
	for _, tc := range []struct {
		name   string
		action control.InputAction
		x      int32
		gap    time.Duration
	}{
		{"inward", control.InputAction{Kind: control.InputPointerMove, DeltaX: -1}, 850, 10 * time.Second},
		{"no_outward_delta", control.InputAction{Kind: control.InputPointerMove}, 850, 10 * time.Second},
		{"non_pointer", control.InputAction{Kind: control.InputButtonDown, Code: 1}, 850, 10 * time.Second},
		{"outside", control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 900, 10 * time.Second},
		{"distant", control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 600, 10 * time.Second},
		{"reversed_clock", control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 850, -time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := previewFixture(control.ScreenEdgeRight)
			p.preview(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 850, at)
			if p.preview(tc.action, tc.x, at.Add(tc.gap)) || !p.sentAt.Equal(at) {
				t.Fatal("ineligible input emitted or refreshed a hint")
			}
		})
	}
}
