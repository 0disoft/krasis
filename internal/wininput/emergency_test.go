package wininput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestEmergencyReleaseDetectorRequiresFullPhysicalChord(t *testing.T) {
	detector := &EmergencyReleaseDetector{}
	sequence := []control.InputAction{
		keyAction(control.InputKeyDown, scanCodeControl),
		keyAction(control.InputKeyDown, scanCodeAlt),
		keyAction(control.InputKeyDown, scanCodeLeftShift),
	}
	for _, action := range sequence {
		if detector.Observe(action) {
			t.Fatalf("detector fired before F12: %+v", action)
		}
	}
	if !detector.Observe(keyAction(control.InputKeyDown, scanCodeF12)) {
		t.Fatal("detector did not fire for Ctrl+Alt+Shift+F12")
	}
	if detector.Observe(keyAction(control.InputKeyDown, scanCodeF12)) {
		t.Fatal("detector remained armed after firing")
	}
}

func TestEmergencyReleaseDetectorAcceptsExtendedControlAndRightShift(t *testing.T) {
	detector := &EmergencyReleaseDetector{}
	detector.Observe(keyAction(control.InputKeyDown, scanCodeControl|extendedScanCodeFlag))
	detector.Observe(keyAction(control.InputKeyDown, scanCodeAlt))
	detector.Observe(keyAction(control.InputKeyDown, scanCodeRightShift))
	if !detector.Observe(keyAction(control.InputKeyDown, scanCodeF12)) {
		t.Fatal("detector did not accept right-side modifier variants")
	}
}

func TestEmergencyReleaseDetectorHonorsModifierRelease(t *testing.T) {
	detector := &EmergencyReleaseDetector{}
	detector.Observe(keyAction(control.InputKeyDown, scanCodeControl))
	detector.Observe(keyAction(control.InputKeyDown, scanCodeAlt))
	detector.Observe(keyAction(control.InputKeyDown, scanCodeLeftShift))
	detector.Observe(keyAction(control.InputKeyUp, scanCodeAlt))
	if detector.Observe(keyAction(control.InputKeyDown, scanCodeF12)) {
		t.Fatal("detector fired after Alt was released")
	}
}

func TestEmergencyBoundaryObserverAbortsActiveSourceWithoutForwardingChordTerminator(t *testing.T) {
	sink := &emergencyRecordingSink{failed: make(chan error, 1)}
	session, err := NewSourceSession(8, sink)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	workerDone := make(chan error, 1)
	ready := make(chan struct{})
	go func() { workerDone <- session.RunWithReady(ctx, func() { close(ready) }) }()
	<-ready
	if err := session.Activate(); err != nil {
		t.Fatal(err)
	}
	next := &emergencyRecordingObserver{}
	observer, err := NewEmergencyBoundaryObserver(session, next)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []control.InputAction{
		keyAction(control.InputKeyDown, scanCodeControl),
		keyAction(control.InputKeyDown, scanCodeAlt),
		keyAction(control.InputKeyDown, scanCodeLeftShift),
	} {
		if err := observer.Observe(action, 0); err != nil {
			t.Fatal(err)
		}
	}
	err = observer.Observe(keyAction(control.InputKeyDown, scanCodeF12), 0)
	if !errors.Is(err, ErrEmergencyRelease) || session.Active() {
		t.Fatalf("emergency result: error=%v active=%t", err, session.Active())
	}
	if len(next.actions) != 3 {
		t.Fatalf("forwarded actions = %d, want 3 modifiers only", len(next.actions))
	}
	select {
	case workerErr := <-workerDone:
		if !errors.Is(workerErr, ErrEmergencyRelease) {
			t.Fatalf("worker error = %v", workerErr)
		}
	case <-time.After(time.Second):
		t.Fatal("source worker did not stop")
	}
}

func keyAction(kind control.InputActionKind, code uint32) control.InputAction {
	return control.InputAction{Kind: kind, Code: code}
}

type emergencyRecordingObserver struct{ actions []control.InputAction }

func (observer *emergencyRecordingObserver) Observe(action control.InputAction, _ int32) error {
	observer.actions = append(observer.actions, action)
	return nil
}

type emergencyRecordingSink struct{ failed chan error }

func (*emergencyRecordingSink) SendAction(context.Context, control.InputAction, func() bool) error {
	return nil
}
func (*emergencyRecordingSink) Interrupt(error) error { return nil }
func (sink *emergencyRecordingSink) Fail(_ context.Context, err error) error {
	sink.failed <- err
	return err
}
