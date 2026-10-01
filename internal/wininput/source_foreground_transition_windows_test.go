//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"time"
)

func stubForegroundTransitionPause(t *testing.T, pause func(time.Duration)) {
	previous := sourceForegroundTransitionPause
	sourceForegroundTransitionPause = pause
	t.Cleanup(func() { sourceForegroundTransitionPause = previous })
}

func TestForegroundFallbackRecoversTransientNullWindow(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	pauses := 0
	stubForegroundTransitionPause(t, func(interval time.Duration) {
		if interval != 10*time.Millisecond {
			t.Fatal("unexpected retry interval")
		}
		pauses++
	})
	reads := 0
	foreground := uintptr(20)
	sourceWheelCaptureGetForeground = func() uintptr {
		reads++
		if reads == 1 {
			return 0
		}
		return foreground
	}
	sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(window uintptr) (uint32, error) {
		if window != 20 {
			t.Fatal("attached an unobserved foreground window")
		}
		return 22, nil
	}
	attached, detached := 0, 0
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if attach {
			attached++
		} else {
			detached++
		}
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10}
	result, err := current.activateForegroundFallback()
	if err != nil || result != SourceForegroundFallbackSucceeded || attached != 1 || detached != 1 || current.attachedSource != 0 || pauses != 1 {
		t.Fatal("transient missing foreground prevented a verified handoff", result, err)
	}
}

func TestForegroundTransitionRemainsBoundedAndCannotUseSavedFocus(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	pauses, reads := 0, 0
	stubForegroundTransitionPause(t, func(time.Duration) { pauses++ })
	sourceWheelCaptureGetForeground = func() uintptr { reads++; return 0 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { t.Fatal("missing foreground used a remembered window"); return 0, nil }
	sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("missing foreground forced activation"); return nil }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("missing foreground attached a thread"); return nil }
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
	result, err := current.activateForegroundFallback()
	if !errors.Is(err, ErrSourceForegroundUnavailable) || result != SourceForegroundFallbackAttachFailed || pauses != 5 || reads != 6 || current.active.Load() {
		t.Fatal("persistent missing foreground bypassed refusal or retry budget", result, err, pauses, reads)
	}
}

func TestForegroundTransitionCancellationStopsBeforeAnotherNativeOperation(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	current := &sourceWheelCaptureRun{window: 10}
	reads, pauses := 0, 0
	sourceWheelCaptureGetForeground = func() uintptr { reads++; return 0 }
	stubForegroundTransitionPause(t, func(time.Duration) { pauses++; current.setError(context.Canceled) })
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("cancelled transition attached a thread"); return nil }
	_, err := current.activateForegroundFallback()
	if !errors.Is(err, context.Canceled) || reads != 1 || pauses != 1 {
		t.Fatal("cancelled transition kept retrying", err)
	}
}

func TestForegroundTransitionAddsNoDelayForAnObservedWindow(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	stubForegroundTransitionPause(t, func(time.Duration) { t.Fatal("normal foreground was delayed") })
	sourceWheelCaptureGetForeground = func() uintptr { return 20 }
	window, err := (&sourceWheelCaptureRun{}).foregroundAfterTransition()
	if window != 20 || err != nil {
		t.Fatal("normal foreground changed", window, err)
	}
}

func TestForegroundTransitionStillRejectsAnUnresponsiveObservedWindow(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	stubForegroundTransitionPause(t, func(time.Duration) {})
	reads := 0
	sourceWheelCaptureGetForeground = func() uintptr {
		reads++
		if reads == 1 {
			return 0
		}
		return 20
	}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureProbeForeground = func(uintptr) error { return errors.New("probe rejected") }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("unresponsive window bypassed probe"); return nil }
	_, err := (&sourceWheelCaptureRun{window: 10}).activateForegroundFallback()
	if !errors.Is(err, ErrSourceForegroundProbeFailed) {
		t.Fatal("native responsiveness check was bypassed", err)
	}
}
