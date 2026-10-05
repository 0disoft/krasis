//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMissingForegroundPreparationTargetsOnlyCaptureAndVerifiesOwner(t *testing.T) {
	for _, owner := range []uintptr{10, 30} {
		t.Run(map[uintptr]string{10: "capture", 30: "external"}[owner], func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			foreground := uintptr(0)
			prepared, dispatched := 0, 0
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceCapturePrepareActive = func(window uintptr) {
				if window != 10 {
					t.Fatal("prepared a remembered or external window")
				}
				prepared++
			}
			sourceForegroundTransitionDispatch = func(uintptr) {
				dispatched++
				if prepared != 0 {
					foreground = owner
				}
			}
			sourceWheelCaptureSetForeground = func(uintptr) error {
				t.Fatal("changed freshly observed owner after queue preparation")
				return nil
			}
			sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error {
				t.Fatal("preparation joined another input queue")
				return nil
			}
			stubForegroundTransitionPause(t, func(time.Duration) { t.Fatal("verified owner was delayed") })
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
			window, err := current.foregroundAfterTransition()
			if err != nil || window != owner || prepared != 1 || dispatched != 2 || current.active.Load() {
				t.Fatal("preparation authorized an unverified capture", window, err, prepared, dispatched)
			}
		})
	}
}

func TestMissingForegroundPreparationCannotRepeatOrAuthorizeFromAPIReturn(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	prepared, requests, pauses := 0, 0, 0
	sourceWheelCaptureGetForeground = func() uintptr { return 0 }
	sourceCapturePrepareActive = func(window uintptr) {
		if window != 10 {
			t.Fatal("unexpected preparation target")
		}
		prepared++ // Simulate native acceptance with no foreground ownership.
	}
	sourceWheelCaptureSetForeground = func(uintptr) error { requests++; return nil }
	stubForegroundTransitionPause(t, func(time.Duration) { pauses++ })
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
	window, err := current.foregroundAfterTransition()
	if window != 0 || !errors.Is(err, ErrSourceForegroundUnavailable) || prepared != 1 || requests != 5 || pauses != 5 || current.active.Load() {
		t.Fatal("preparation bypassed verification or retry budget", window, err, prepared, requests, pauses)
	}
}

func TestMissingForegroundPreparationCancellationStopsDispatchAndRetry(t *testing.T) {
	for _, duringDispatch := range []bool{false, true} {
		restore := stubSourceWheelCaptureActivation(t)
		current := &sourceWheelCaptureRun{window: 10}
		prepared, dispatched := 0, 0
		sourceWheelCaptureGetForeground = func() uintptr { return 0 }
		sourceCapturePrepareActive = func(uintptr) {
			prepared++
			if !duringDispatch {
				current.setError(context.Canceled)
			}
		}
		sourceForegroundTransitionDispatch = func(uintptr) {
			dispatched++
			if duringDispatch && prepared != 0 {
				current.setError(context.Canceled)
			}
		}
		sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("cancelled preparation retried"); return nil }
		window, err := current.foregroundAfterTransition()
		restore()
		wantDispatches := 1
		if duringDispatch {
			wantDispatches = 2
		}
		if window != 0 || !errors.Is(err, context.Canceled) || prepared != 1 || dispatched != wantDispatches || current.active.Load() {
			t.Fatal("preparation ignored retirement", window, err, prepared, dispatched)
		}
	}
}

func TestMissingForegroundPreparationExcludesExistingOwnerAndPendingAttachment(t *testing.T) {
	for _, test := range []struct {
		name          string
		owner, window uintptr
		attached      uint32
	}{
		{"external", 30, 10, 0},
		{"capture", 10, 10, 0},
		{"no_window", 0, 0, 0},
		{"pending_attachment", 0, 10, 11},
	} {
		t.Run(test.name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			sourceWheelCaptureGetForeground = func() uintptr { return test.owner }
			sourceCapturePrepareActive = func(uintptr) { t.Fatal("prepared an ineligible queue") }
			sourceWheelCaptureSetForeground = func(uintptr) error { return nil }
			stubForegroundTransitionPause(t, func(time.Duration) {})
			current := &sourceWheelCaptureRun{window: test.window, attachedSource: test.attached, savedFocus: 20}
			window, err := current.foregroundAfterTransition()
			if window != test.owner || (test.owner == 0) != errors.Is(err, ErrSourceForegroundUnavailable) || current.active.Load() {
				t.Fatal("ineligible preparation changed existing outcome", window, err)
			}
		})
	}
}
