//go:build windows

package wininput

import (
	"errors"
	"testing"
)

func TestCaptureCleanRejectionKeepsOwnerReusable(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(20)
	deny := true
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		if deny {
			return errors.New("denied")
		}
		foreground = window
		return nil
	}
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return errors.New("denied") }
	previousCursor := callSetSourceCaptureCursor
	callSetSourceCaptureCursor = func(...uintptr) (uintptr, uintptr, error) { return 0, 0, nil }
	defer func() { callSetSourceCaptureCursor = previousCursor }()
	current := &sourceWheelCaptureRun{window: 10, commands: make(chan sourceWheelCaptureCommand, 1)}
	run := func() error {
		result := make(chan error, 1)
		current.commands <- sourceWheelCaptureCommand{message: wmSourceWheelCaptureActivate, result: result}
		current.handleCommand(wmSourceWheelCaptureActivate)
		return <-result
	}
	err := run()
	if _, ok := err.(*sourceCaptureActivationRejected); !ok || current.err() != nil || current.active.Load() || current.registered {
		t.Fatalf("clean rejection poisoned owner: err=%v terminal=%v", err, current.err())
	}
	deny = false
	if err := run(); err != nil || !current.active.Load() || current.err() != nil {
		t.Fatalf("retry failed: %v terminal=%v", err, current.err())
	}
}

func TestCaptureRollbackRequiresEveryCleanupProof(t *testing.T) {
	for _, name := range []string{"clean", "unregister", "visible", "foreground", "detach", "terminal"} {
		t.Run(name, func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			current := &sourceWheelCaptureRun{window: 10, registered: true}
			sourceWheelCaptureGetForeground = func() uintptr { return 20 }
			cause := ErrSourceCaptureActivationAPIRejected
			switch name {
			case "unregister":
				sourceWheelCaptureRegisterWindow = func(uintptr, uintptr, bool) error { return errors.New("unregister failed") }
			case "visible":
				sourceWheelCaptureIsVisible = func(uintptr) bool { return true }
			case "foreground":
				sourceWheelCaptureGetForeground = func() uintptr { return 10 }
			case "detach":
				cause = ErrSourceCaptureThreadDetachFailed
			case "terminal":
				current.setError(errors.New("deadline exceeded"))
			}
			err := current.rollbackActivation(cause)
			_, clean := err.(*sourceCaptureActivationRejected)
			if clean != (name == "clean") || !errors.Is(err, cause) {
				t.Fatalf("invalid cleanup proof: %v", err)
			}
			if name != "clean" && !errors.Is(err, ErrSourceCaptureRollbackFailed) {
				t.Fatalf("missing rollback failure: %v", err)
			}
		})
	}
}

func TestCaptureAlreadyForegroundDoesNotAttachToItself(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	sourceWheelCaptureGetForeground = func() uintptr { return 10 }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("unexpected attach"); return nil }
	result, err := (&sourceWheelCaptureRun{window: 10}).activateForegroundFallback()
	if err != nil || result != SourceForegroundFallbackSucceeded {
		t.Fatalf("result=%v err=%v", result, err)
	}
}

func TestCapturePartialActivationRollbackAcceptsVerifiedFocusRelease(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "released", true: "retained"}[retained], func(t *testing.T) {
			restore := stubSourceWheelCaptureActivation(t)
			defer restore()
			foreground := uintptr(10)
			sourceWheelCaptureGetForeground = func() uintptr { return foreground }
			sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("focus denied") }
			sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
			sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
			sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return errors.New("attach denied") }
			sourceWheelCaptureShowWindow = func(_ uintptr, command uintptr) {
				if command == swHide && !retained {
					foreground = 20
				}
			}
			// Activation registered the window but failed before publishing active.
			current := &sourceWheelCaptureRun{window: 10, savedFocus: 20, registered: true}
			err := current.rollbackActivation(ErrSourceCaptureActivationVerificationLost)
			_, rejected := err.(*sourceCaptureActivationRejected)
			if rejected == retained {
				t.Fatalf("retained=%v rollback=%v", retained, err)
			}
			if current.active.Load() || current.registered || current.savedFocus != 0 {
				t.Fatal("rollback retained ownership")
			}
		})
	}
}
