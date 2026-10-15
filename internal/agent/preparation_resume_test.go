package agent

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestPreparationResumesBeforeEdgeBandAndRetriesBusyQueue(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		p := previewFixture(edge)
		p.hints = make(chan time.Time, 1)
		at := time.Unix(100, 0)
		p.movementAt = at // Session setup or the last physical movement.
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
		x := int32(100) // 799px from the right edge, outside the prediction band.
		if edge == control.ScreenEdgeLeft {
			x, move.DeltaX = 700, -1
		}
		p.hints <- at
		resumed := at.Add(10 * time.Second)
		if p.offerPreparation(move, x, resumed) || !p.sentAt.IsZero() {
			t.Fatal("full queue accepted an idle-resume hint")
		}
		<-p.hints
		if !p.offerPreparation(move, x, resumed.Add(time.Millisecond)) {
			t.Fatal("idle first movement lost preparation before the edge band")
		}
		<-p.hints
		if p.offerPreparation(move, x, resumed.Add(2*time.Millisecond)) || len(p.hints) != 0 {
			t.Fatal("resume burst launched more than one preparation")
		}
		// Arrival at the edge uses that candidate instead of sending again.
		if edge == control.ScreenEdgeRight {
			x = 850
		} else {
			x = -50
		}
		if p.offerPreparation(move, x, resumed.Add(100*time.Millisecond)) {
			t.Fatal("the same resumed approach emitted another hint")
		}
	}
}

func TestPreparationResumeRetryExpiresAndDoesNotExtendWithMovement(t *testing.T) {
	p := previewFixture(control.ScreenEdgeRight)
	p.hints = make(chan time.Time, 1)
	at := time.Unix(100, 0)
	p.movementAt = at
	move := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	p.hints <- at
	resumed := at.Add(10 * time.Second)
	p.offerPreparation(move, 100, resumed)
	p.offerPreparation(move, 100, resumed.Add(200*time.Millisecond))
	<-p.hints
	if p.offerPreparation(move, 100, resumed.Add(preparationHintLifetime)) {
		t.Fatal("continuous movement extended a missed idle-resume hint")
	}
	if len(p.hints) != 0 {
		t.Fatal("expired resume retained queued work")
	}
}
