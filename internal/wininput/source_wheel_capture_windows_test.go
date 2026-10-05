//go:build windows

package wininput

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceWheelCaptureShutdownDoesNotPromoteCancellationToFailure(t *testing.T) {
	for _, cause := range []error{context.Canceled, errors.Join(context.Canceled, errors.New("native cleanup failed"))} {
		current := &sourceWheelCaptureRun{terminalErr: cause}
		capture := &sourceWheelCapture{run: current}
		err := capture.execute(wmSourceWheelCaptureDeactivate, true)
		if cancellationOnly(err) != cancellationOnly(cause) {
			t.Fatalf("shutdown changed failure classification: cause=%v result=%v", cause, err)
		}
	}
}

func TestSourceWheelCaptureRestoresFocusBeforeHiding(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	t.Cleanup(restore)
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
	want := []string{"get", "validate", "restore", "get", "get", "hide", "get"}
	if !slices.Equal(events, want) {
		t.Fatalf("deactivation order = %v, want %v", events, want)
	}
}

func TestSourceWheelCaptureUsesBoundedFocusRestoreFallback(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	foreground := uintptr(10)
	setCalls := 0
	attachCalls := make([]bool, 0, 2)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		setCalls++
		if setCalls == 1 {
			return errors.New("restore denied")
		}
		foreground = window
		return nil
	}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(window uintptr) (uint32, error) {
		if window != 20 {
			t.Fatalf("focus fallback peer window = %d, want 20", window)
		}
		return 22, nil
	}
	sourceWheelCaptureAttachThreads = func(sourceThread, peerThread uint32, attach bool) error {
		if sourceThread != 11 || peerThread != 22 {
			t.Fatalf("AttachThreadInput(%d, %d), want (11, 22)", sourceThread, peerThread)
		}
		attachCalls = append(attachCalls, attach)
		return nil
	}

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, observer: observer}
	current.active.Store(true)
	if err := current.deactivateWindow(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(attachCalls, []bool{true, false}) {
		t.Fatalf("AttachThreadInput calls = %v, want [true false]", attachCalls)
	}
	if !slices.Equal(observer.focusFallbacks, []SourceFocusRestoreFallbackResult{SourceFocusRestoreFallbackSucceeded}) {
		t.Fatalf("focus fallback results = %v", observer.focusFallbacks)
	}
	if !slices.Equal(observer.focusRestoreResults, []SourceFocusRestoreResult{SourceFocusRestoreSucceeded}) {
		t.Fatalf("focus restore results = %v", observer.focusRestoreResults)
	}
}

func TestSourceWheelCaptureHidesAfterFocusRestoreFallbackAttachFailure(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	showCalled := false
	foreground := uintptr(10)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("restore denied") }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return errors.New("attach denied") }
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) {
		showCalled = true
		foreground = 30
	}

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, observer: observer}
	current.active.Store(true)
	if err := current.deactivateWindow(); err != nil || !showCalled {
		t.Fatalf("deactivateWindow() error=%v showCalled=%t", err, showCalled)
	}
	if !slices.Equal(observer.focusFallbacks, []SourceFocusRestoreFallbackResult{SourceFocusRestoreFallbackAttachFailed}) {
		t.Fatalf("focus fallback results = %v", observer.focusFallbacks)
	}
	if !slices.Equal(observer.focusRestoreResults, []SourceFocusRestoreResult{SourceFocusRestoreAPIRejected}) {
		t.Fatalf("focus restore results = %v", observer.focusRestoreResults)
	}
	if !slices.Equal(observer.postHideFocusResults, []SourcePostHideFocusResult{SourcePostHideFocusRecovered}) {
		t.Fatalf("post-hide focus results = %v", observer.postHideFocusResults)
	}
}

func TestSourceWheelCaptureClassifiesFocusRestoreVerificationLoss(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	foreground := uintptr(10)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(uintptr) error {
		foreground = 30
		return nil
	}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return errors.New("attach denied") }
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) {}

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, observer: observer}
	current.active.Store(true)
	if err := current.deactivateWindow(); err != nil {
		t.Fatalf("deactivateWindow() error=%v", err)
	}
	if !slices.Equal(observer.focusRestoreResults, []SourceFocusRestoreResult{SourceFocusRestoreVerificationLost}) {
		t.Fatalf("focus restore results = %v", observer.focusRestoreResults)
	}
	if !slices.Equal(observer.focusFallbacks, []SourceFocusRestoreFallbackResult{SourceFocusRestoreFallbackAttachFailed}) {
		t.Fatalf("focus fallback results = %v", observer.focusFallbacks)
	}
	if !slices.Equal(observer.postHideFocusResults, []SourcePostHideFocusResult{SourcePostHideFocusRecovered}) {
		t.Fatalf("post-hide focus results = %v", observer.postHideFocusResults)
	}
}

func TestSourceWheelCaptureClassifiesFocusRestoreFallbackDetachFailure(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	foreground := uintptr(10)
	setCalls := 0
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		setCalls++
		if setCalls == 1 {
			return errors.New("restore denied")
		}
		foreground = window
		return nil
	}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if !attach {
			return errors.New("detach denied")
		}
		return nil
	}

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, observer: observer}
	current.active.Store(true)
	err := current.deactivateWindow()
	if !errors.Is(err, ErrSourceFocusRestoreThreadDetachFailed) {
		t.Fatalf("deactivateWindow() error = %v", err)
	}
	if !slices.Equal(observer.focusFallbacks, []SourceFocusRestoreFallbackResult{SourceFocusRestoreFallbackDetachFailed}) {
		t.Fatalf("focus fallback results = %v", observer.focusFallbacks)
	}
	if !slices.Equal(observer.postHideFocusResults, []SourcePostHideFocusResult{SourcePostHideFocusRecovered}) {
		t.Fatalf("post-hide focus results = %v", observer.postHideFocusResults)
	}
}

func TestSourceWheelCaptureAllowsNoForegroundAfterDegradedRestore(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	foreground := uintptr(10)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("restore denied") }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if attach {
			return errors.New("attach denied")
		}
		return nil
	}
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) { foreground = 0 }

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, observer: observer}
	current.active.Store(true)
	if err := current.deactivateWindow(); err != nil {
		t.Fatalf("deactivateWindow() error = %v", err)
	}
	if !slices.Equal(observer.postHideFocusResults, []SourcePostHideFocusResult{SourcePostHideFocusUnavailable}) {
		t.Fatalf("post-hide focus results = %v", observer.postHideFocusResults)
	}
}

func TestSourceWheelCaptureFailsWhenHiddenCaptureRetainsForeground(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	sourceWheelCaptureGetForeground = func() uintptr { return 10 }
	sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("restore denied") }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if attach {
			return errors.New("attach denied")
		}
		return nil
	}
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) {}

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, observer: observer}
	current.active.Store(true)
	if err := current.deactivateWindow(); !errors.Is(err, ErrSourceFocusReleaseCaptureRetained) {
		t.Fatalf("deactivateWindow() error = %v", err)
	}
	if !slices.Equal(observer.postHideFocusResults, []SourcePostHideFocusResult{SourcePostHideFocusCaptureRetained}) {
		t.Fatalf("post-hide focus results = %v", observer.postHideFocusResults)
	}
}

func TestSourceWheelCaptureClassifiesFocusRestoreFallbackRetryFailure(t *testing.T) {
	for _, test := range []struct {
		name       string
		setErr     error
		foreground uintptr
		wantResult SourceFocusRestoreFallbackResult
		wantError  error
	}{
		{name: "API rejected", setErr: errors.New("restore denied"), foreground: 10, wantResult: SourceFocusRestoreFallbackAPIRejected, wantError: ErrSourceFocusRestoreAPIRejected},
		{name: "verification lost", foreground: 10, wantResult: SourceFocusRestoreFallbackVerificationLost, wantError: ErrSourceFocusRestoreVerificationLost},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			sourceWheelCaptureGetForeground = func() uintptr { return test.foreground }
			sourceWheelCaptureSetForeground = func(uintptr) error { return test.setErr }
			sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
			sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
			sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return nil }

			result, err := (&sourceWheelCaptureRun{window: 10, savedFocus: 20}).restoreFocusFallback()
			if result != test.wantResult || !errors.Is(err, test.wantError) {
				t.Fatalf("restoreFocusFallback() = (%d, %v), want (%d, %v)", result, err, test.wantResult, test.wantError)
			}
		})
	}
}

func TestSourceWheelCaptureDoesNotRecordFocusForFailedActivationCleanup(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	t.Cleanup(restore)
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

func TestSourceWheelCaptureClassifiesActivationSetupFailure(t *testing.T) {
	previousPosition := sourceWheelCaptureSetPosition
	previousRegister := sourceWheelCaptureRegisterWindow
	previousShow := sourceWheelCaptureShowWindow
	t.Cleanup(func() {
		sourceWheelCaptureSetPosition = previousPosition
		sourceWheelCaptureRegisterWindow = previousRegister
		sourceWheelCaptureShowWindow = previousShow
	})

	sourceWheelCaptureRegisterWindow = func(uintptr, uintptr, bool) error { return nil }
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) {}
	sourceWheelCaptureSetPosition = func(uintptr, int32, int32) error { return errors.New("position denied") }
	current := &sourceWheelCaptureRun{window: 10}
	if err := current.activateWindow(); !errors.Is(err, ErrSourceCaptureActivationSetup) {
		t.Fatalf("activateWindow() error = %v, want %v", err, ErrSourceCaptureActivationSetup)
	}
}

func TestSourceWheelCaptureUsesBoundedForegroundFallback(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	foreground := uintptr(20)
	setCalls := 0
	attachCalls := make([]bool, 0, 2)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		setCalls++
		if setCalls == 1 {
			return errors.New("foreground denied")
		}
		foreground = window
		return nil
	}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(window uintptr) (uint32, error) {
		if window != 20 {
			t.Fatalf("fallback foreground window = %d, want 20", window)
		}
		return 22, nil
	}
	sourceWheelCaptureAttachThreads = func(sourceThread, targetThread uint32, attach bool) error {
		if sourceThread != 11 || targetThread != 22 {
			t.Fatalf("AttachThreadInput(%d, %d), want (11, 22)", sourceThread, targetThread)
		}
		attachCalls = append(attachCalls, attach)
		return nil
	}

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, observer: observer}
	if err := current.activateWindow(); err != nil {
		t.Fatal(err)
	}
	if !current.active.Load() || observer.captureActivations != 1 {
		t.Fatalf("capture activation state = active:%t observer:%+v", current.active.Load(), observer)
	}
	if !slices.Equal(attachCalls, []bool{true, false}) {
		t.Fatalf("AttachThreadInput calls = %v, want [true false]", attachCalls)
	}
	if !slices.Equal(observer.foregroundFallbacks, []SourceForegroundFallbackResult{SourceForegroundFallbackSucceeded}) {
		t.Fatalf("foreground fallback results = %v", observer.foregroundFallbacks)
	}
}

func TestSourceWheelCaptureAbortsWhenForegroundFallbackAttachFails(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	registered := make([]bool, 0, 2)
	sourceWheelCaptureRegisterWindow = func(_ uintptr, _ uintptr, enabled bool) error {
		registered = append(registered, enabled)
		return nil
	}
	sourceWheelCaptureGetForeground = func() uintptr { return 20 }
	sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("foreground denied") }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return errors.New("attach denied") }

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, observer: observer}
	err := current.activateWindow()
	if !errors.Is(err, ErrSourceCaptureActivationAPIRejected) || !errors.Is(err, ErrSourceCaptureThreadAttachFailed) {
		t.Fatalf("activateWindow() error = %v", err)
	}
	if current.active.Load() || current.registered || !slices.Equal(registered, []bool{true, false}) {
		t.Fatalf("failed activation was not rolled back: active=%t registered=%t calls=%v", current.active.Load(), current.registered, registered)
	}
	if !slices.Equal(observer.foregroundFallbacks, []SourceForegroundFallbackResult{SourceForegroundFallbackAttachFailed}) {
		t.Fatalf("foreground fallback results = %v", observer.foregroundFallbacks)
	}
}

func TestSourceWheelCaptureAbortsWhenForegroundFallbackDetachFails(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()

	foreground := uintptr(20)
	setCalls := 0
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		setCalls++
		if setCalls == 1 {
			return errors.New("foreground denied")
		}
		foreground = window
		return nil
	}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if !attach {
			return errors.New("detach denied")
		}
		return nil
	}

	observer := &recordingSourcePipelineObserver{}
	current := &sourceWheelCaptureRun{window: 10, observer: observer}
	err := current.activateWindow()
	if !errors.Is(err, ErrSourceCaptureThreadDetachFailed) {
		t.Fatalf("activateWindow() error = %v", err)
	}
	if current.active.Load() || current.registered {
		t.Fatalf("failed activation was not rolled back: active=%t registered=%t", current.active.Load(), current.registered)
	}
	if !slices.Equal(observer.foregroundFallbacks, []SourceForegroundFallbackResult{SourceForegroundFallbackDetachFailed}) {
		t.Fatalf("foreground fallback results = %v", observer.foregroundFallbacks)
	}
}

func TestSourceWheelCaptureClassifiesForegroundFallbackRetryFailure(t *testing.T) {
	for _, test := range []struct {
		name       string
		setErr     error
		foreground uintptr
		wantResult SourceForegroundFallbackResult
		wantError  error
	}{
		{name: "API rejected", setErr: errors.New("foreground denied"), foreground: 20, wantResult: SourceForegroundFallbackAPIRejected, wantError: ErrSourceCaptureActivationAPIRejected},
		{name: "verification lost", foreground: 20, wantResult: SourceForegroundFallbackVerificationLost, wantError: ErrSourceCaptureActivationVerificationLost},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			sourceWheelCaptureGetForeground = func() uintptr { return test.foreground }
			sourceWheelCaptureSetForeground = func(uintptr) error { return test.setErr }
			sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
			sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
			sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return nil }

			result, err := (&sourceWheelCaptureRun{window: 10}).activateForegroundFallback()
			if result != test.wantResult || !errors.Is(err, test.wantError) {
				t.Fatalf("activateForegroundFallback() = (%d, %v), want (%d, %v)", result, err, test.wantResult, test.wantError)
			}
		})
	}
}

func stubSourceWheelCaptureActivation(t *testing.T) func() {
	t.Helper()
	previousPrepare := sourceCapturePrepareActive
	sourceCapturePrepareActive = func(uintptr) {}
	previousDispatch := sourceForegroundTransitionDispatch
	sourceForegroundTransitionDispatch = func(uintptr) {}
	previousVisible := sourceWheelCaptureIsVisible
	sourceWheelCaptureIsVisible = func(uintptr) bool { return false }
	previousProbe := sourceWheelCaptureProbeForeground
	sourceWheelCaptureProbeForeground = func(uintptr) error { return nil }
	previousPosition := sourceWheelCaptureSetPosition
	previousRegister := sourceWheelCaptureRegisterWindow
	previousShow := sourceWheelCaptureShowWindow
	previousHide := sourceWheelCaptureHideWithoutActivation
	previousGet := sourceWheelCaptureGetForeground
	previousSet := sourceWheelCaptureSetForeground
	previousIsWindow := sourceWheelCaptureIsWindow
	previousCurrentThread := sourceWheelCaptureCurrentThread
	previousWindowThread := sourceWheelCaptureWindowThread
	previousAttachThreads := sourceWheelCaptureAttachThreads
	sourceWheelCaptureSetPosition = func(uintptr, int32, int32) error { return nil }
	sourceWheelCaptureRegisterWindow = func(uintptr, uintptr, bool) error { return nil }
	sourceWheelCaptureShowWindow = func(uintptr, uintptr) {}
	sourceWheelCaptureHideWithoutActivation = func(window uintptr) error {
		sourceWheelCaptureShowWindow(window, swHide)
		return nil
	}
	sourceWheelCaptureIsWindow = func(uintptr) bool { return true }
	return func() {
		sourceCapturePrepareActive = previousPrepare
		sourceForegroundTransitionDispatch = previousDispatch
		sourceWheelCaptureIsVisible = previousVisible
		sourceWheelCaptureProbeForeground = previousProbe
		sourceWheelCaptureSetPosition = previousPosition
		sourceWheelCaptureRegisterWindow = previousRegister
		sourceWheelCaptureShowWindow = previousShow
		sourceWheelCaptureHideWithoutActivation = previousHide
		sourceWheelCaptureGetForeground = previousGet
		sourceWheelCaptureSetForeground = previousSet
		sourceWheelCaptureIsWindow = previousIsWindow
		sourceWheelCaptureCurrentThread = previousCurrentThread
		sourceWheelCaptureWindowThread = previousWindowThread
		sourceWheelCaptureAttachThreads = previousAttachThreads
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
		t.Fatalf("retired capture message faulted session: %v", cause)
	default:
	}
}

func TestTerminalCaptureCommandCannotActivate(t *testing.T) {
	cause := errors.New("expired capture generation")
	current := &sourceWheelCaptureRun{commands: make(chan sourceWheelCaptureCommand, 1)}
	current.active.Store(true)
	current.setError(cause)
	result := make(chan error, 1)
	current.commands <- sourceWheelCaptureCommand{message: wmSourceWheelCaptureActivate, result: result}
	current.handleCommand(wmSourceWheelCaptureActivate)
	if err := <-result; !errors.Is(err, cause) {
		t.Fatalf("late command result: %v", err)
	}
	if current.active.Load() {
		t.Fatal("terminal capture reactivated")
	}
	if err := current.activateWindow(); !errors.Is(err, cause) {
		t.Fatalf("late activation: %v", err)
	}
}

func TestCaptureLossStopsSuppressionBeforeCleanupAndRejectsLateActivation(t *testing.T) {
	session, err := NewSourceSession(1, &recordingSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	// Leave the fault worker stopped to prove that retirement is synchronous.
	session.started.Store(true)
	if err = session.Activate(); err != nil {
		t.Fatal(err)
	}
	current := &sourceWheelCaptureRun{session: session}
	current.active.Store(true)
	if !current.loseForeground() {
		t.Fatal("active loss not reported")
	}
	if session.Active() || session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("lost capture retained suppression")
	}
	if err = session.Activate(); err == nil {
		t.Fatal("late activation revived lost capture")
	}
	if cause := <-session.faults; !errors.Is(cause, ErrSourceCaptureLostForeground) {
		t.Fatal(cause)
	}
	if current.loseForeground() {
		t.Fatal("normal inactive notification escalated")
	}
}

func TestCaptureSerializationHasDeadline(t *testing.T) {
	previous := sourceWheelCaptureTimeout
	sourceWheelCaptureTimeout = 10 * time.Millisecond
	t.Cleanup(func() { sourceWheelCaptureTimeout = previous })
	capture := newSourceWheelCapture(nil)
	capture.opOnce.Do(func() { capture.opGate = make(chan struct{}, 1) })
	capture.opGate <- struct{}{}
	start := time.Now()
	if err := capture.Activate(); err == nil || !strings.Contains(err.Error(), "serialization timed out") {
		t.Fatalf("Activate: %v", err)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("serialization was not bounded")
	}
}

func packedWindowWheel(delta int16) uintptr {
	return uintptr(uint32(uint16(delta)) << 16)
}
func TestSourceCaptureHidesCursorOnlyWhileActive(t *testing.T) {
	original := callSetSourceCaptureCursor
	t.Cleanup(func() { callSetSourceCaptureCursor = original })
	calls := 0
	callSetSourceCaptureCursor = func(args ...uintptr) (uintptr, uintptr, error) {
		calls++
		if len(args) != 1 || args[0] != 0 {
			t.Fatal("expected null cursor")
		}
		return 0, 0, nil
	}
	current := &sourceWheelCaptureRun{}
	if current.hideActiveCursor() || calls != 0 {
		t.Fatal("inactive capture hid cursor")
	}
	current.active.Store(true)
	for range 3 {
		if !current.hideActiveCursor() {
			t.Fatal("active cursor request was not handled")
		}
	}
	current.active.Store(false)
	if current.hideActiveCursor() || calls != 3 {
		t.Fatal("cursor hiding survived deactivation")
	}
}
