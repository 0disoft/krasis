package wininput

import (
	"errors"
	"sync"

	"github.com/0disoft/krasis/internal/control"
)

var ErrEmergencyRelease = errors.New("local emergency release requested")

const (
	scanCodeLeftShift  uint32 = 0x2A
	scanCodeRightShift uint32 = 0x36
	scanCodeControl    uint32 = 0x1D
	scanCodeAlt        uint32 = 0x38
	scanCodeF12        uint32 = 0x58
)

type EmergencyReleaseDetector struct {
	mu         sync.Mutex
	leftShift  bool
	rightShift bool
	control    bool
	alt        bool
}

func (detector *EmergencyReleaseDetector) Observe(action control.InputAction) bool {
	if detector == nil || (action.Kind != control.InputKeyDown && action.Kind != control.InputKeyUp) {
		return false
	}
	pressed := action.Kind == control.InputKeyDown
	scanCode := action.Code & scanCodeMask
	detector.mu.Lock()
	defer detector.mu.Unlock()
	switch scanCode {
	case scanCodeLeftShift:
		detector.leftShift = pressed
	case scanCodeRightShift:
		detector.rightShift = pressed
	case scanCodeControl:
		detector.control = pressed
	case scanCodeAlt:
		detector.alt = pressed
	case scanCodeF12:
		if pressed && detector.control && detector.alt && (detector.leftShift || detector.rightShift) {
			detector.reset()
			return true
		}
	}
	return false
}

func (detector *EmergencyReleaseDetector) reset() {
	detector.leftShift = false
	detector.rightShift = false
	detector.control = false
	detector.alt = false
}

type EmergencyBoundaryObserver struct {
	detector *EmergencyReleaseDetector
	session  *SourceSession
	next     BoundaryObserver
}

func NewEmergencyBoundaryObserver(session *SourceSession, next BoundaryObserver) (*EmergencyBoundaryObserver, error) {
	if session == nil || next == nil {
		return nil, errors.New("source session and boundary observer are required")
	}
	return &EmergencyBoundaryObserver{detector: &EmergencyReleaseDetector{}, session: session, next: next}, nil
}

func (observer *EmergencyBoundaryObserver) Observe(action control.InputAction, pointerX int32) error {
	if observer == nil || observer.detector == nil || observer.session == nil || observer.next == nil {
		return errors.New("emergency boundary observer is not initialized")
	}
	if observer.detector.Observe(action) {
		observer.session.Abort(ErrEmergencyRelease)
		return ErrEmergencyRelease
	}
	return observer.next.Observe(action, pointerX)
}
