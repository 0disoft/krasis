//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
)

func TestFocusRestoreWaitsForRequestedQueueBeforeHiding(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(10)
	pending := uintptr(0)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error { pending = window; foreground = 0; return nil }
	sourceWheelCaptureProbeForeground = func(window uintptr) error {
		if window != 20 || pending != window {
			t.Fatal("wait did not address the requested restore queue")
		}
		foreground = pending
		return nil
	}
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("asynchronous restore joined input queues"); return nil }
	sourceWheelCaptureHideWithoutActivation = func(uintptr) error {
		if foreground != 20 {
			t.Fatal("capture hidden before restore completed")
		}
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, registered: true}
	if err := current.deactivateWindow(); err != nil || foreground != 20 || current.savedFocus != 0 {
		t.Fatal("restore left the foreground transition unfinished", err)
	}
}

func TestForegroundFallbackCancellationDuringDetachCannotAuthorize(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	current := &sourceWheelCaptureRun{window: 10}
	foreground := uintptr(20)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if !attach {
			current.setError(context.Canceled)
		}
		return nil
	}
	result, err := current.setForegroundWithThreadAttach(10, 20)
	if result != foregroundThreadFallbackVerificationLost || !errors.Is(err, context.Canceled) || current.attachedSource != 0 {
		t.Fatal("detachment cancellation authorized a retired capture", result, err)
	}
}

func TestForegroundFallbackRechecksOwnershipAfterDetach(t *testing.T) {
	for _, finish := range []bool{true, false} {
		name := "completed"
		if !finish {
			name = "lost"
		}
		t.Run(name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			foreground := uintptr(10)
			detached, waits := false, 0
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
			sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
			sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
			sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
				if !attach {
					detached = true
					foreground = 0
				}
				return nil
			}
			sourceWheelCaptureProbeForeground = func(window uintptr) error {
				if detached {
					waits++
					if finish {
						foreground = window
					} else {
						foreground = 30
					}
				}
				return nil
			}
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
			result, err := current.setForegroundWithThreadAttach(20, 20)
			if waits != 1 || current.attachedSource != 0 || (err == nil) != finish || (result == foregroundThreadFallbackSucceeded) != finish {
				t.Fatal("joined-queue observation survived detach without final proof", result, err, waits)
			}
		})
	}
}

func TestForegroundFallbackCompletionTimeoutCannotUseEarlierProof(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(10)
	detached := false
	cause := errors.New("bounded completion timeout")
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(_, _ uint32, attach bool) error {
		if !attach {
			detached = true
			foreground = 0
		}
		return nil
	}
	sourceWheelCaptureProbeForeground = func(uintptr) error {
		if detached {
			return cause
		}
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10}
	result, err := current.setForegroundWithThreadAttach(20, 20)
	if result != foregroundThreadFallbackVerificationLost || !errors.Is(err, cause) || current.attachedSource != 0 {
		t.Fatal("timeout reused ownership observed before detach", result, err)
	}
}
