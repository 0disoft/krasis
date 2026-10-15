package agent

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestPreparationSameApproachRenewsAfterLaunchCooldownBeforeColdEntryGap(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		p := previewFixture(edge)
		p.hints = make(chan time.Time, 1)
		x, dx := int32(850), int32(1)
		if edge == control.ScreenEdgeLeft {
			x, dx = -50, -1
		}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: dx}
		at := time.Unix(100, 0)
		if !p.offerPreparation(move, x, at) {
			t.Fatal("initial hint missing")
		}
		<-p.hints
		// The source remains physically active near the same edge. It must
		// not wait for a retreat or the legacy 2.5s suppression interval.
		if p.offerPreparation(move, x, at.Add(1900*time.Millisecond)) {
			t.Fatal("launch cooldown bypassed")
		}
		if !p.offerPreparation(move, x, at.Add(2200*time.Millisecond)) {
			t.Fatal("fresh movement retained the cold-entry gap")
		}
		<-p.hints
		if p.offerPreparation(move, x, at.Add(2300*time.Millisecond)) || len(p.hints) != 0 {
			t.Fatal("one approach flooded preparation")
		}
	}
}
