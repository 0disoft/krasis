//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCaptureActivationCompletesSentForegroundRequestWithoutClick(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	stubForegroundTransitionPause(t, func(time.Duration) {})
	foreground := uintptr(0)
	pending := uintptr(0)
	dispatches := 0
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		pending = window // Request returns before Windows completes activation.
		return nil
	}
	sourceForegroundTransitionDispatch = func(window uintptr) {
		if window != 10 || pending != window {
			t.Fatal("unrequested window activation")
		}
		dispatches++
		foreground = pending
	}
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error {
		t.Fatal("NULL recovery attached an unknown thread")
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10}
	if err := current.activateWindow(); err != nil || !current.active.Load() || dispatches != 1 {
		t.Fatal("pending activation still required a click", err, dispatches)
	}
}

func TestForegroundDispatchCancellationCannotAuthorizeOrRetry(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	stubForegroundTransitionPause(t, func(time.Duration) {})
	current := &sourceWheelCaptureRun{window: 10}
	reads, dispatches := 0, 0
	sourceWheelCaptureGetForeground = func() uintptr { reads++; return 0 }
	sourceForegroundTransitionDispatch = func(uintptr) { dispatches++; current.setError(context.Canceled) }
	sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("cancelled activation retried"); return nil }
	_, err := current.foregroundAfterTransition()
	if !errors.Is(err, context.Canceled) || dispatches != 1 || reads != 1 || current.active.Load() {
		t.Fatal("sent-message cancellation was ignored", err, dispatches, reads)
	}
}

func TestForegroundDispatchDoesNotRemoveOrDispatchQueuedMessages(t *testing.T) {
	previous := callSourceForegroundMessages
	defer func() { callSourceForegroundMessages = previous }()
	calls := 0
	callSourceForegroundMessages = func(args ...uintptr) (uintptr, uintptr, error) {
		calls++
		if len(args) != 5 || args[0] == 0 || args[1] != 10 || args[2] != 0 || args[3] != 0 || args[4] != 0x00400000 {
			t.Fatal("foreground wait admitted queued input or removed shutdown/commands")
		}
		return 0, 0, nil // No queued message is not an API failure.
	}
	sourceForegroundTransitionDispatch(0)
	sourceForegroundTransitionDispatch(10)
	if calls != 1 {
		t.Fatal("dispatch used an unknown owner or repeated the native call")
	}
}

func TestCaptureActivationCompletesRequestWhilePreviousOwnerRemainsVisible(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground, pending := uintptr(20), uintptr(0)
	dispatches := 0
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error { pending = window; return nil }
	sourceForegroundTransitionDispatch = func(window uintptr) {
		if window != 10 || pending != window {
			t.Fatal("serviced an unrequested activation")
		}
		dispatches++
		foreground = pending
	}
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error {
		t.Fatal("joined the previous app's queue before completing our activation")
		return nil
	}
	stubForegroundTransitionPause(t, func(time.Duration) { t.Fatal("pending non-NULL activation was delayed") })
	current := &sourceWheelCaptureRun{window: 10}
	if err := current.activateWindow(); err != nil || !current.active.Load() || foreground != 10 || current.savedFocus != 20 || dispatches != 1 {
		t.Fatal("pending activation did not retain verified ownership and restore target", err, dispatches)
	}
}

func TestCaptureActivationCancellationDuringPendingRequestDoesNotAttach(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	current := &sourceWheelCaptureRun{window: 10}
	sourceWheelCaptureGetForeground = func() uintptr { return 20 }
	sourceWheelCaptureSetForeground = func(uintptr) error { return nil }
	sourceForegroundTransitionDispatch = func(uintptr) { current.setError(context.Canceled) }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("cancelled activation joined a queue"); return nil }
	if err := current.activateWindow(); !errors.Is(err, context.Canceled) || current.active.Load() {
		t.Fatal("cancelled pending request became active", err)
	}
}

func TestCaptureActivationCancellationDuringVerificationStopsBeforeDispatch(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	current := &sourceWheelCaptureRun{window: 10}
	requested := false
	sourceWheelCaptureSetForeground = func(uintptr) error { requested = true; return nil }
	sourceWheelCaptureGetForeground = func() uintptr {
		if requested {
			current.setError(context.Canceled)
		}
		return 20
	}
	sourceForegroundTransitionDispatch = func(uintptr) { t.Fatal("retired verification serviced another native request") }
	if err := current.activateWindow(); !errors.Is(err, context.Canceled) || current.active.Load() {
		t.Fatal("verification cancellation was ignored", err)
	}
}
