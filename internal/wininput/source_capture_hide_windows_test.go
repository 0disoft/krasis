//go:build windows

package wininput

import (
	"errors"
	"syscall"
	"testing"
)

func TestCaptureRepeatedHandoffPreservesRestoredForeground(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(20)
	hides := 0
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
	sourceWheelCaptureShowWindow = func(_ uintptr, command uintptr) {
		if command == swHide {
			t.Fatal("return asked Windows to choose another active window")
		}
	}
	sourceWheelCaptureHideWithoutActivation = func(window uintptr) error {
		if window != 10 || foreground != 20 {
			t.Fatal("hide did not preserve the observed external owner")
		}
		hides++
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10}
	for i := 0; i < 20; i++ {
		if err := current.activateWindow(); err != nil {
			t.Fatal("next handoff needed a physical click", i, err)
		}
		if err := current.deactivateWindow(); err != nil {
			t.Fatal("return failed", i, err)
		}
		if foreground != 20 || current.active.Load() || current.registered {
			t.Fatal("return lost focus or retained capture", i)
		}
	}
	if hides != 20 {
		t.Fatal("return hide was skipped", hides)
	}
}

func TestCaptureHideRejectsFailureOrHiddenForegroundRetention(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "API rejected", true: "hidden capture retained"}[retained], func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			foreground := uintptr(10)
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
			sourceWheelCaptureHideWithoutActivation = func(uintptr) error {
				if retained {
					foreground = 10
					return nil
				}
				return errors.New("hide denied")
			}
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
			current.active.Store(true)
			err := current.deactivateWindow()
			want := ErrSourceCaptureRollbackFailed
			if retained {
				want = ErrSourceFocusReleaseCaptureRetained
			}
			if !errors.Is(err, want) || current.active.Load() {
				t.Fatal("hide failed without surfacing unsafe cleanup", err)
			}
		})
	}
}

func TestCaptureHideNativeFlagsAndError(t *testing.T) {
	previous := callSourceCaptureHide
	t.Cleanup(func() { callSourceCaptureHide = previous })
	for _, denied := range []bool{false, true} {
		callSourceCaptureHide = func(args ...uintptr) (uintptr, uintptr, error) {
			if len(args) != 7 || args[0] != 10 || args[6] != 0x97 {
				t.Fatal("native hide changed position, size, z-order or activation", args)
			}
			for _, value := range args[1:6] {
				if value != 0 {
					t.Fatal("native hide used another window or geometry")
				}
			}
			if denied {
				return 0, 0, syscall.ERROR_ACCESS_DENIED
			}
			return 1, 0, syscall.Errno(0)
		}
		err := sourceWheelCaptureHideWithoutActivation(10)
		if (err != nil) != denied {
			t.Fatal("native hide result was ignored", err)
		}
	}
}
