package wininput

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func queuedMove(x, y int32) *queuedAction {
	item := &queuedAction{action: control.InputAction{Kind: control.InputPointerMove, DeltaX: x, DeltaY: y}}
	item.move.Store(packQueuedMove(x, y))
	return item
}

func TestMoveClaimAndMergeCannotLoseOrSplitAxes(t *testing.T) {
	for range 1000 {
		item := queuedMove(-3, 5)
		start := make(chan struct{})
		merged := make(chan bool, 1)
		go func() { <-start; merged <- item.mergeMove(2, -4) }()
		close(start)
		action, ok := item.takeAction()
		if !ok {
			t.Fatal("single consumer could not claim move")
		}
		x, y := int32(-3), int32(5)
		if <-merged {
			x, y = -1, 1
		}
		if action.DeltaX != x || action.DeltaY != y || item.mergeMove(1, 1) {
			t.Fatal("claim raced merge with lost or torn axes", action)
		}
		if _, again := item.takeAction(); again {
			t.Fatal("claimed move was replayed")
		}
	}
}

func TestMoveMergeRetainsRangeCancellationAndNoAllocation(t *testing.T) {
	for _, test := range []struct{ x, y, dx, dy int32 }{
		{control.MaxInputDelta, 0, 1, 0},
		{-control.MaxInputDelta, 0, -1, 0},
		{0, control.MaxInputDelta, 0, 1},
		{0, -control.MaxInputDelta, 0, -1},
		{1, -1, -1, 1},
	} {
		item := queuedMove(test.x, test.y)
		if item.mergeMove(test.dx, test.dy) {
			t.Fatal("out-of-range or canceled move was merged")
		}
		action, ok := item.takeAction()
		if !ok || action.DeltaX != test.x || action.DeltaY != test.y {
			t.Fatal("rejected merge changed original input", action)
		}
	}
	item := queuedMove(1, -1)
	allocations := testing.AllocsPerRun(1000, func() {
		item.move.Store(packQueuedMove(1, -1))
		if !item.mergeMove(1, -1) {
			t.Fatal("valid move did not merge")
		}
	})
	if allocations != 0 {
		t.Fatal("hot-path coalescing added allocations", allocations)
	}
}

func TestProducerAfterMoveClaimQueuesSeparateFrame(t *testing.T) {
	s := activatedOfferSession(t)
	if !s.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("initial move rejected")
	}
	first := <-s.queue
	action, ok := first.takeAction()
	if !ok || action.DeltaX != 1 {
		t.Fatal("initial claim lost input")
	}
	// pendingMove deliberately still points to the claimed item. Producers own
	// that pointer; the sender must not wait for their mutex to clear it.
	if !s.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaY: -2}) || len(s.queue) != 1 {
		t.Fatal("new move was merged into an already claimed frame")
	}
	second := <-s.queue
	action, ok = second.takeAction()
	if second == first || !ok || action.DeltaX != 0 || action.DeltaY != -2 {
		t.Fatal("post-claim frame changed", action)
	}
}

func TestSourceSenderDoesNotWaitForProducerQueueMutex(t *testing.T) {
	s := activatedOfferSession(t)
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1)}
	s.sink = sink
	if !s.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("initial move rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	s.queueMu.Lock()
	defer func() { s.queueMu.Unlock(); cancel(); <-done }()
	go func() { done <- s.runSender(ctx) }()
	select {
	case action := <-sink.sent:
		if action.DeltaX != 1 {
			t.Fatal("sender lost queued move", action)
		}
	case <-time.After(time.Second):
		t.Fatal("sender waited for the producer queue mutex")
	}
}
