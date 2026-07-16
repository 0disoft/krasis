//go:build windows

package wininput

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceWheelCaptureRestoresFocusBeforeHiding(t *testing.T) {
	previousShow := sourceWheelCaptureShowWindow
	previousGet := sourceWheelCaptureGetForeground
	previousSet := sourceWheelCaptureSetForeground
	previousIsWindow := sourceWheelCaptureIsWindow
	t.Cleanup(func() {
		sourceWheelCaptureShowWindow = previousShow
		sourceWheelCaptureGetForeground = previousGet
		sourceWheelCaptureSetForeground = previousSet
		sourceWheelCaptureIsWindow = previousIsWindow
	})

	foreground := uintptr(10)
	events := make([]string, 0, 5)
	sourceWheelCaptureGetForeground = func() uintptr {
		events = append(events, "get")
		return foreground
	}
	sourceWheelCaptureIsWindow = func(window uintptr) bool {
		events = append(events, "validate")
		return window == 20
	}
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		events = append(events, "restore")
		foreground = window
		return nil
	}
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) {
		events = append(events, "hide")
	}

	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
	current.active.Store(true)
	if err := current.deactivateWindow(); err != nil {
		t.Fatal(err)
	}
	want := []string{"get", "validate", "restore", "get", "hide"}
	if !slices.Equal(events, want) {
		t.Fatalf("deactivation order = %v, want %v", events, want)
	}
}

func TestSourceWheelCaptureHidesAfterFocusRestoreFailure(t *testing.T) {
	previousShow := sourceWheelCaptureShowWindow
	previousGet := sourceWheelCaptureGetForeground
	previousSet := sourceWheelCaptureSetForeground
	previousIsWindow := sourceWheelCaptureIsWindow
	t.Cleanup(func() {
		sourceWheelCaptureShowWindow = previousShow
		sourceWheelCaptureGetForeground = previousGet
		sourceWheelCaptureSetForeground = previousSet
		sourceWheelCaptureIsWindow = previousIsWindow
	})

	showCalled := false
	sourceWheelCaptureGetForeground = func() uintptr { return 10 }
	sourceWheelCaptureIsWindow = func(uintptr) bool { return true }
	sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("restore denied") }
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) { showCalled = true }

	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
	current.active.Store(true)
	if err := current.deactivateWindow(); err == nil || !showCalled {
		t.Fatalf("deactivateWindow() error=%v showCalled=%t", err, showCalled)
	}
}

func TestSourceWheelCaptureDoesNotRecordFocusForFailedActivationCleanup(t *testing.T) {
	previousRegister := sourceWheelCaptureRegisterWindow
	previousShow := sourceWheelCaptureShowWindow
	previousGet := sourceWheelCaptureGetForeground
	previousSet := sourceWheelCaptureSetForeground
	previousIsWindow := sourceWheelCaptureIsWindow
	t.Cleanup(func() {
		sourceWheelCaptureRegisterWindow = previousRegister
		sourceWheelCaptureShowWindow = previousShow
		sourceWheelCaptureGetForeground = previousGet
		sourceWheelCaptureSetForeground = previousSet
		sourceWheelCaptureIsWindow = previousIsWindow
	})

	foreground := uintptr(10)
	sourceWheelCaptureRegisterWindow = func(uintptr, uintptr, bool) error { return nil }
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) {}
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		foreground = window
		return nil
	}
	sourceWheelCaptureIsWindow = func(uintptr) bool { return true }
	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{
		window: 10, savedFocus: 20, registered: true, observer: observer,
	}
	if err := current.deactivateWindow(); err != nil {
		t.Fatal(err)
	}
	if observer.captureDeactivations != 0 || len(observer.focusRestoreResults) != 0 {
		t.Fatalf("failed activation cleanup was recorded as lifecycle evidence: %+v", observer)
	}
}

func TestSourceWheelCaptureCommandAdmissionTimeoutAlsoBoundsRollback(t *testing.T) {
	previous := sourceWheelCaptureTimeout
	sourceWheelCaptureTimeout = 10 * time.Millisecond
	t.Cleanup(func() { sourceWheelCaptureTimeout = previous })
	capture := newSourceWheelCapture(nil)
	current := &sourceWheelCaptureRun{
		window:   1,
		commands: make(chan sourceWheelCaptureCommand, 1),
		done:     make(chan struct{}),
	}
	current.commands <- sourceWheelCaptureCommand{}
	if err := capture.attach(current); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { capture.detach(current) })
	started := time.Now()
	err := capture.Activate()
	if err == nil || !strings.Contains(err.Error(), "admission timed out") {
		t.Fatalf("Activate() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("Activate() was not bounded: %s", elapsed)
	}
	started = time.Now()
	capture.Deactivate()
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("rollback Deactivate() was not bounded: %s", elapsed)
	}
}

func TestMapWindowMouseWheelPreservesSignedAxes(t *testing.T) {
	vertical, emitted, err := mapWindowMouseWheel(wmMouseWheel, packedWindowWheel(-120))
	if err != nil || !emitted || vertical.Kind != control.InputWheel || vertical.DeltaX != 0 || vertical.DeltaY != -120 {
		t.Fatalf("vertical = %+v emitted=%t err=%v", vertical, emitted, err)
	}
	horizontal, emitted, err := mapWindowMouseWheel(wmMouseHWheel, packedWindowWheel(240))
	if err != nil || !emitted || horizontal.Kind != control.InputWheel || horizontal.DeltaX != 240 || horizontal.DeltaY != 0 {
		t.Fatalf("horizontal = %+v emitted=%t err=%v", horizontal, emitted, err)
	}
	if action, emitted, err := mapWindowMouseWheel(wmMouseWheel, 0); err != nil || emitted || action.Kind != "" {
		t.Fatalf("zero delta = %+v emitted=%t err=%v", action, emitted, err)
	}
	if action, emitted, err := mapWindowMouseWheel(wmMouseMove, packedWindowWheel(120)); err != nil || emitted || action.Kind != "" {
		t.Fatalf("unknown message = %+v emitted=%t err=%v", action, emitted, err)
	}
}

func TestSourceWheelCaptureRoutesOneWindowMessageToOneSourceAction(t *testing.T) {
	observer := &recordingSourcePipelineObserver{}
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1), failed: make(chan error, 1)}
	session := startObservedSourceSession(t, 2, sink, observer)
	current := &sourceWheelCaptureRun{session: session, observer: observer}
	current.routeWheel(wmMouseWheel, packedWindowWheel(120))
	action := receiveAction(t, sink.sent)
	if action.Kind != control.InputWheel || action.DeltaY != 120 || action.DeltaX != 0 {
		t.Fatalf("source action = %+v", action)
	}
	if observer.windowObserved != 1 || observer.mapped != 1 || observer.queueAccepted != 1 {
		t.Fatalf("observer = %+v", observer)
	}
}

func TestSourceWheelCaptureRejectsWheelWhenSourceIsInactive(t *testing.T) {
	observer := &recordingSourcePipelineObserver{}
	session, err := NewSourceSessionObserved(1, &recordingSourceSink{}, observer)
	if err != nil {
		t.Fatal(err)
	}
	current := &sourceWheelCaptureRun{session: session, observer: observer}
	current.routeWheel(wmMouseWheel, packedWindowWheel(120))
	if observer.windowObserved != 1 || observer.mapped != 1 || observer.queueAccepted != 0 {
		t.Fatalf("observer = %+v", observer)
	}
	select {
	case cause := <-session.faults:
		if cause == nil {
			t.Fatal("inactive capture fault was nil")
		}
	default:
		t.Fatal("inactive capture did not fault the source session")
	}
}

func packedWindowWheel(delta int16) uintptr {
	return uintptr(uint32(uint16(delta)) << 16)
}
