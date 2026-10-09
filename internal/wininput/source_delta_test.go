package wininput

import (
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

func TestSourceDeltaLimitPreservesTotalsAndButtonOrder(t *testing.T) {
	for _, single := range []bool{false, true} {
		block := make(chan struct{})
		sink := &recordingSourceSink{blockSend: block, sent: make(chan control.InputAction, 8)}
		session := startSourceSession(t, 6, sink)
		if !session.Offer(control.InputAction{Kind: control.InputWheel, DeltaY: 120}) {
			t.Fatal("initial admission")
		}
		receiveAction(t, sink.sent)
		moves := []control.InputAction{{Kind: control.InputPointerMove, DeltaX: 20000, DeltaY: -20000}, {Kind: control.InputPointerMove, DeltaX: 20000, DeltaY: -20000}}
		if single {
			moves = []control.InputAction{{Kind: control.InputPointerMove, DeltaX: 40000, DeltaY: -40000}}
		}
		down := control.InputAction{Kind: control.InputButtonDown, Code: 1}
		up := control.InputAction{Kind: control.InputButtonUp, Code: 1}
		for _, action := range append(moves, down, up) {
			if !session.Offer(action) {
				t.Fatal("admission failed", action)
			}
		}
		close(block)
		var x, y int32
		for i := 0; i < 2; i++ {
			a := receiveAction(t, sink.sent)
			if a.Kind != control.InputPointerMove || a.DeltaX > control.MaxInputDelta || a.DeltaY < -control.MaxInputDelta {
				t.Fatal(a)
			}
			x += a.DeltaX
			y += a.DeltaY
		}
		if x != 40000 || y != -40000 || receiveAction(t, sink.sent) != down || receiveAction(t, sink.sent) != up {
			t.Fatal("totals or order lost", x, y)
		}
	}
}
