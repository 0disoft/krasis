package residentagent

import (
	"github.com/0disoft/krasis/internal/peer"
	"time"
)

const boundaryNoticeLifetime = 3 * time.Second

func (observer *sessionObserver) ObserveControlFlow(event peer.ControlFlowEvent) {
	code := boundaryFlowCode(event)
	if code == "" && event != peer.ControlFlowCaptureActivationSucceeded {
		// Most lease progress notifications have no UI/counter effect here.
		// Do not serialize grant authorization and frame dispatch behind an
		// unrelated Operations owner just to return without changing state.
		return
	}
	o := observer.operations
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sessionDone != observer.done || o.root.Err() != nil || o.sessionState != "CONNECTED" {
		return
	}
	if code != "" {
		o.recordLocked(code)
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

// Keep the actual admission/write result after the brief UI notice expires.
// These fixed phase labels contain no key, pointer, window or peer data.
func boundaryFlowCode(event peer.ControlFlowEvent) string {
	switch event {
	case peer.ControlFlowBoundaryDroppedNotReady:
		return "boundary_dropped_not_ready"
	case peer.ControlFlowBoundaryBlockedLocalMenu:
		return "boundary_blocked_menu"
	case peer.ControlFlowBoundaryBlockedMouseCapture:
		return "boundary_blocked_capture"
	case peer.ControlFlowBoundaryBlockedPressedInput:
		return "boundary_blocked_pressed"
	case peer.ControlFlowBoundaryBlockedMoveSize:
		return "boundary_blocked_move_size"
	case peer.ControlFlowBoundaryAdmissionFailed:
		return "boundary_admission_failed"
	case peer.ControlFlowBoundaryAdmitted:
		return "boundary_admitted"
	case peer.ControlFlowRequestWriteCompleted:
		return "boundary_request_written"
	case peer.ControlFlowRequestWriteOutcomeUnknown:
		return "boundary_request_unknown"
	default:
		return ""
	}
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
