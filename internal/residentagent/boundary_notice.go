package residentagent

import (
	"github.com/0disoft/krasis/internal/peer"
	"time"
)

const boundaryNoticeLifetime = 3 * time.Second

func (observer *sessionObserver) ObserveControlFlow(event peer.ControlFlowEvent) {
	o := observer.operations
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sessionDone != observer.done || o.root.Err() != nil || o.sessionState != "CONNECTED" {
		return
	}
	var reason string
	switch event {
	case peer.ControlFlowBoundaryBlockedLocalMenu:
		reason = "LOCAL_MENU"
	case peer.ControlFlowBoundaryBlockedMouseCapture:
		reason = "MOUSE_CAPTURE"
	case peer.ControlFlowBoundaryBlockedPressedInput:
		reason = "PRESSED_INPUT"
	case peer.ControlFlowBoundaryBlockedMoveSize:
		reason = "MOVE_SIZE"
	case peer.ControlFlowBoundaryAdmitted, peer.ControlFlowCaptureActivationSucceeded:
		o.boundaryBlocked, o.boundaryBlockedAt = "", time.Time{}
		return
	default:
		return
	}
	o.boundaryBlocked, o.boundaryBlockedAt = reason, o.now()
}

// Caller holds mu. A past blockage is not presented as a current failure.
func (o *Operations) boundaryNoticeLocked() string {
	if o.stateLocked() != "CONNECTED" || o.boundaryBlockedAt.IsZero() {
		return ""
	}
	age := o.now().Sub(o.boundaryBlockedAt)
	if age < 0 || age >= boundaryNoticeLifetime {
		return ""
	}
	return o.boundaryBlocked
}
