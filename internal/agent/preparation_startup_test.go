package agent

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestPreparationModerateApproachLeavesTheExistingPreparationBudget(t *testing.T) {
	for _, edge := range []control.ScreenEdge{control.ScreenEdgeLeft, control.ScreenEdgeRight} {
		p := previewFixture(edge) // An ongoing approach, not the initial resume.
		p.hints = make(chan time.Time, 1)
		at := time.Unix(100, 0)
		start, delta := int32(400), int32(1)
		if edge == control.ScreenEdgeLeft {
			delta = -1
		}
		move := control.InputAction{Kind: control.InputPointerMove, DeltaX: delta}
		var hinted time.Time
		var edgeAt time.Time
		for step := int32(0); step <= 520; step += 20 {
			x := start + delta*step // One physical pixel per millisecond.
			now := at.Add(time.Duration(step) * time.Millisecond)
			if p.offerPreparation(move, x, now) && hinted.IsZero() {
				hinted = now
			}
			if edge == control.ScreenEdgeRight && x >= 899 || edge == control.ScreenEdgeLeft && x <= -100 {
				edgeAt = now
				break
			}
		}
		if hinted.IsZero() || edgeAt.IsZero() || hinted.Add(InputPreparationTimeout-InputPreparationEntryWait).After(edgeAt) {
			t.Fatalf("moderate approach exceeds the bounded entry wait: hint=%v edge=%v", hinted.Sub(at), edgeAt.Sub(at))
		}
	}
}
