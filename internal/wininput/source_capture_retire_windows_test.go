//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
)

func TestCaptureRetirementAvoidsExternalFocusCalls(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "loop exit", true: "cancelled"}[cancelled], func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, registered: true}
			current.active.Store(true)
			if cancelled {
				current.recordError(context.Canceled, false)
			}
			sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("retirement requested external focus"); return nil }
			sourceWheelCaptureProbeForeground = func(uintptr) error { t.Fatal("retirement probed external queue"); return nil }
			sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("retirement attached external queue"); return nil }
			sourceWheelCaptureShowWindow = func(uintptr, uintptr) { t.Fatal("retirement used activating hide") }
			hides, unregisters := 0, 0
			sourceWheelCaptureHideWithoutActivation = func(w uintptr) error {
				if w != 10 {
					t.Fatal("wrong owned window")
				}
				hides++
				return nil
			}
			sourceWheelCaptureRegisterWindow = func(_, w uintptr, register bool) error {
				if w != 10 || register {
					t.Fatal("wrong unregister")
				}
				unregisters++
				return nil
			}
			if err := current.retireWindow(); err != nil {
				t.Fatal(err)
			}
			if current.active.Load() || current.registered || current.savedFocus != 0 || hides != 1 || unregisters != 1 {
				t.Fatal("retirement retained native ownership")
			}
			if err := current.retireWindow(); err != nil || hides != 1 || unregisters != 1 {
				t.Fatal("retirement was not idempotent", err)
			}
		})
	}
}

func TestCaptureRetirementPreservesNativeCleanupFailures(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	current := &sourceWheelCaptureRun{window: 10, registered: true, attachedSource: 11, attachedPeer: 22}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	detachErr, unregisterErr, hideErr := errors.New("detach"), errors.New("unregister"), errors.New("hide")
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if attach {
			t.Fatal("cleanup reattached")
		}
		return detachErr
	}
	sourceWheelCaptureRegisterWindow = func(uintptr, uintptr, bool) error { return unregisterErr }
	sourceWheelCaptureHideWithoutActivation = func(uintptr) error { return hideErr }
	err := current.retireWindow()
	for _, want := range []error{detachErr, unregisterErr, hideErr} {
		if !errors.Is(err, want) {
			t.Fatal("lost native failure", want, err)
		}
	}
	if !current.registered || current.attachedSource != 11 || current.attachedPeer != 22 {
		t.Fatal("lost cleanup obligation")
	}
}
