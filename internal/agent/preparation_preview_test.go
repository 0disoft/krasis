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

// Model successful emission for the geometry/cooldown tests; queue rejection
// is covered separately through the production offerPreparation path.
func (p *preparationBoundary) previewAndSend(action control.InputAction, x int32, now time.Time) bool {
	if !p.preview(action, x, now) {
		return false
	}
	p.preparationHintSent(now)
	return true
}

func TestPreparationFullQueueDoesNotConsumeInitialOrRenewedHint(t *testing.T) {
	for _, renewal := range []bool{false, true} {
		p := previewFixture(control.ScreenEdgeRight)
		p.hints = make(chan time.Time, 1)
		at := time.Unix(100, 0)
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		if renewal {
			if !p.offerPreparation(move, 850, at) {
				t.Fatal("initial hint missing")
			}
			<-p.hints
		}
		oldAt := p.sentAt
		p.hints <- at // Busy sender: hook-side queue admission must not wait.
		now := at.Add(preparationHintRenewal)
		if p.offerPreparation(move, 850, now) || !p.sentAt.Equal(oldAt) {
			t.Fatal("rejected queue admission consumed the approach cooldown")
		}
		<-p.hints
		if !p.offerPreparation(move, 850, now.Add(time.Millisecond)) {
			t.Fatal("fresh physical movement could not retry queue admission")
		}
		if len(p.hints) != 1 || !p.sentAt.Equal(now.Add(time.Millisecond)) || p.offerPreparation(move, 850, now.Add(2*time.Millisecond)) {
			t.Fatal("successful emission lost suppression or bounded the queue incorrectly")
		}
	}
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
		if p.previewAndSend(move, start, now) {
			t.Fatal("unknown initial speed caused distant preparation")
		}
		if !p.previewAndSend(move, approach, now.Add(20*time.Millisecond)) {
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
	p.previewAndSend(move, 100, now)
	if p.previewAndSend(move, 102, now.Add(20*time.Millisecond)) || p.band != 192 {
		t.Fatal("raw counts were interpreted as physical velocity")
	}
	if !p.previewAndSend(move, 850, now.Add(time.Second)) {
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
		if p.previewAndSend(move, start, at) {
			t.Fatal("initial sample prepared before speed was known")
		}
		// 2px/ms reaches the existing three-quarter-screen prediction cap.
		// Preparation remains bounded even with the longer lead.
		if !p.previewAndSend(move, next, at.Add(20*time.Millisecond)) || p.band != 750 {
			t.Fatal("moderate approach retained the last-moment hint", p.band)
		}
	}
}

func TestPreparationPredictionDropsStaleReversedAndOutOfDisplaySamples(t *testing.T) {
	now := time.Unix(100, 0)
	for _, gap := range []time.Duration{-time.Millisecond, previewSampleMaximum + time.Millisecond} {
		p := previewFixture(control.ScreenEdgeRight)
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		p.previewAndSend(move, 100, now)
		if p.previewAndSend(move, 250, now.Add(gap)) || p.band != 192 {
			t.Fatal("stale/reversed clock produced predictive preparation")
		}
	}
	p := previewFixture(control.ScreenEdgeRight)
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	p.previewAndSend(move, 100, now)
	if p.previewAndSend(move, 50, now.Add(20*time.Millisecond)) || p.band != 192 {
		t.Fatal("physical reversal retained outward speed")
	}
	for _, x := range []int32{-101, 900} {
		if p.previewAndSend(move, x, now.Add(25*time.Millisecond)) || !p.sampleAt.IsZero() {
			t.Fatal("outside-display position was predicted")
		}
	}
}

func TestPreparationDoesNotRepeatWhenPredictionFalls(t *testing.T) {
	p := previewFixture(control.ScreenEdgeRight)
	now := time.Unix(100, 0)
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	p.previewAndSend(move, 100, now)
	if !p.previewAndSend(move, 250, now.Add(20*time.Millisecond)) {
		t.Fatal("initial predictive preparation missing")
	}
	if p.previewAndSend(move, 700, now.Add(time.Second)) || p.rearmAt != 782 {
		t.Fatal("decayed speed estimate rearmed an outward approach")
	}
	away := control.InputAction{Kind: control.InputPointerMove, DeltaX: -1}
	if p.previewAndSend(away, 0, now.Add(2*time.Second)) || p.sent {
		t.Fatal("real retreat did not rearm preparation")
	}
	if !p.previewAndSend(move, 850, now.Add(3*time.Second)) {
		t.Fatal("next approach did not prepare")
	}
}

func TestPreparationRetreatRemainsReachableOnNarrowScreens(t *testing.T) {
	now := time.Unix(100, 0)
	for _, width := range []int32{2, 8, 100, 1000} {
		p := &preparationBoundary{config: control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: width, RearmDistance: width - 1}}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		if !p.previewAndSend(move, width-1, now) {
			t.Fatal("near-edge preparation missing")
		}
		if p.previewAndSend(control.InputAction{Kind: control.InputPointerMove, DeltaX: -1}, 0, now.Add(time.Second)) || p.sent {
			t.Fatal("valid retreat could never rearm preparation")
		}
	}
}

func TestPreparationPredictionSmoothsVeryShortDuplicateSamples(t *testing.T) {
	p := previewFixture(control.ScreenEdgeRight)
	now := time.Unix(100, 0)
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	p.previewAndSend(move, 100, now)
	if p.previewAndSend(move, 250, now.Add(time.Microsecond)) || p.band != 192 {
		t.Fatal("sub-millisecond sample caused an unbounded speed estimate")
	}
	if !p.previewAndSend(move, 251, now.Add(4*time.Millisecond)) {
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
		if !p.previewAndSend(move, x, at) {
			t.Fatal("initial approach did not prepare")
		}
		if p.previewAndSend(move, x, at.Add(preparationHintRenewal-time.Nanosecond)) {
			t.Fatal("renewal ignored the cooldown margin")
		}
		if !p.previewAndSend(move, x, at.Add(preparationHintRenewal)) {
			t.Fatal("expired hint still required a distant retreat")
		}
		if p.previewAndSend(move, x, at.Add(preparationHintRenewal+time.Millisecond)) {
			t.Fatal("renewal repeated on consecutive input")
		}
		if !p.previewAndSend(move, x, at.Add(10*time.Second)) {
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
		{"active_distant", control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 600, time.Second},
		{"reversed_clock", control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 850, -time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := previewFixture(control.ScreenEdgeRight)
			p.previewAndSend(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 850, at)
			if p.previewAndSend(tc.action, tc.x, at.Add(tc.gap)) || !p.sentAt.Equal(at) {
				t.Fatal("ineligible input emitted or refreshed a hint")
			}
		})
	}
}
