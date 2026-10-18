package agent

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestPreparationRetreatedApproachRenewsBeforeEdgeDuringContinuousUse(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		p := previewFixture(edge)
		p.hints = make(chan time.Time, 1)
		at := time.Unix(100, 0)
		near, retreat, next, dx := int32(850), int32(0), int32(100), int32(1)
		if edge == control.ScreenEdgeLeft {
			near, retreat, next, dx = -50, 800, 700, -1
		}
		outward := control.InputAction{Kind: control.InputPointerMove, DeltaX: dx}
		inward := control.InputAction{Kind: control.InputPointerMove, DeltaX: -dx}
		if !p.offerPreparation(outward, near, at) {
			t.Fatal("initial hint missing")
		}
		<-p.hints
		// Keep producing physical movement: this is not an idle-resume hint.
		for _, elapsed := range []time.Duration{time.Second, 2 * time.Second, preparationHintRenewal} {
			if p.offerPreparation(inward, retreat, at.Add(elapsed)) || p.sent {
				t.Fatal("retreat launched work or did not rearm")
			}
		}
		// Busy hook-side admission must not consume the early renewal.
		p.hints <- at
		if p.offerPreparation(outward, next, at.Add(preparationHintRenewal+time.Millisecond)) || !p.sentAt.Equal(at) {
			t.Fatal("busy queue consumed renewal")
		}
		<-p.hints
		if !p.offerPreparation(outward, next, at.Add(preparationHintRenewal+2*time.Millisecond)) {
			t.Fatal("continuous reapproach waited for the edge band")
		}
		<-p.hints
		if p.offerPreparation(outward, near, at.Add(preparationHintRenewal+100*time.Millisecond)) {
			t.Fatal("same approach repeated early preparation")
		}
	}
}

func TestPreparationEarlyRenewalRequiresRetreatAndCooldown(t *testing.T) {
	at := time.Unix(100, 0)
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	p := previewFixture(control.ScreenEdgeRight)
	if !p.previewAndSend(move, 710, at) {
		t.Fatal("initial hint missing")
	}
	// Still inside the original retreat threshold, outside the fixed band.
	for i := 1; i <= 3; i++ {
		if p.previewAndSend(move, 690, at.Add(time.Duration(i)*time.Second)) {
			t.Fatal("expired hint launched distant work without retreat")
		}
	}
	p = previewFixture(control.ScreenEdgeRight)
	p.previewAndSend(move, 850, at)
	p.previewAndSend(control.InputAction{Kind: control.InputPointerMove, DeltaX: -1}, 0, at.Add(time.Second))
	if p.previewAndSend(move, 100, at.Add(2*time.Second)) {
		t.Fatal("early renewal bypassed sender/receiver cooldown")
	}
}
