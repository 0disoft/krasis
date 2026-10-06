package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/peer"
	"testing"
	"time"
)

func TestBoundaryNoticeExpiresClearsAndIgnoresRetiredRunner(t *testing.T) {
	now := time.Unix(100, 0)
	done := make(chan struct{})
	o := &Operations{root: context.Background(), now: func() time.Time { return now }, sessionDone: done, sessionState: "CONNECTED"}
	observer := &sessionObserver{operations: o, done: done}
	for _, event := range []peer.ControlFlowEvent{peer.ControlFlowBoundaryBlockedLocalMenu, peer.ControlFlowBoundaryBlockedMouseCapture, peer.ControlFlowBoundaryBlockedPressedInput, peer.ControlFlowBoundaryBlockedMoveSize} {
		observer.ObserveControlFlow(event)
		status, _ := o.Status(context.Background())
		if status.BoundaryBlocked == "" {
			t.Fatal("blocked reason missing")
		}
		observer.ObserveControlFlow(peer.ControlFlowBoundaryAdmitted)
		status, _ = o.Status(context.Background())
		if status.BoundaryBlocked != "" {
			t.Fatal("admitted boundary kept stale notice")
		}
	}
	observer.ObserveControlFlow(peer.ControlFlowBoundaryBlockedLocalMenu)
	now = now.Add(boundaryNoticeLifetime)
	status, _ := o.Status(context.Background())
	if status.BoundaryBlocked != "" {
		t.Fatal("expired notice retained")
	}
	o.sessionDone = make(chan struct{})
	observer.ObserveControlFlow(peer.ControlFlowBoundaryBlockedPressedInput)
	status, _ = o.Status(context.Background())
	if status.BoundaryBlocked != "" {
		t.Fatal("retired runner changed current notice")
	}
}
