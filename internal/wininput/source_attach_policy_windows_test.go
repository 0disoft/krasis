//go:build windows

package wininput

import (
	"errors"
	"testing"
)

func TestProductForegroundAttachmentPairsNativeFlags(t *testing.T) {
	previous := callSourceAttachThreadInput
	t.Cleanup(func() { callSourceAttachThreadInput = previous })
	var flags []uintptr
	callSourceAttachThreadInput = func(args ...uintptr) (uintptr, uintptr, error) {
		if args[0] != 11 || args[1] != 22 {
			t.Fatal("wrong thread identities")
		}
		flags = append(flags, args[2])
		return 1, 0, nil
	}
	if err := sourceWheelCaptureAttachThreads(11, 22, true); err != nil {
		t.Fatal(err)
	}
	if err := sourceWheelCaptureAttachThreads(11, 22, false); err != nil {
		t.Fatal(err)
	}
	if len(flags) != 2 || flags[0] != 1 || flags[1] != 0 {
		t.Fatalf("flags: %v", flags)
	}
}

func TestUnresponsiveForegroundNeverAttaches(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	cause := errors.New("foreground probe timeout")
	sourceWheelCaptureProbeForeground = func(uintptr) error { return cause }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { t.Fatal("attached unresponsive foreground"); return nil }
	result, err := (&sourceWheelCaptureRun{}).setForegroundWithThreadAttach(1, 2)
	if result != foregroundThreadFallbackAttachFailed || !errors.Is(err, cause) {
		t.Fatalf("result=%v error=%v", result, err)
	}
}

func TestRetiredSourceRejectsLateActivation(t *testing.T) {
	session, _ := NewSourceSession(1, &recordingSourceSink{})
	session.started.Store(true)
	session.physicalMu.Lock()
	done := make(chan error, 1)
	go func() { done <- session.Activate() }()
	session.Retire()
	session.physicalMu.Unlock()
	if err := <-done; err == nil || session.Active() {
		t.Fatal("retired source reactivated")
	}
}

func TestForegroundProbeHasFixedDeadline(t *testing.T) {
	previous := callSourceForegroundProbe
	t.Cleanup(func() { callSourceForegroundProbe = previous })
	callSourceForegroundProbe = func(args ...uintptr) (uintptr, uintptr, error) {
		if len(args) != 7 || args[0] != 42 || args[1] != 0 || args[4] != 0x23 || args[5] != 100 || args[6] != 0 {
			t.Fatalf("unbounded probe: %v", args)
		}
		return 1, 0, nil
	}
	if err := sourceWheelCaptureProbeForeground(42); err != nil {
		t.Fatal(err)
	}
}

func TestRetiredForegroundFallbackStillDetaches(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	current := &sourceWheelCaptureRun{}
	cause := errors.New("capture retired")
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureGetForeground = func() uintptr { return 0 }
	sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("retired run changed foreground"); return nil }
	detached := false
	sourceWheelCaptureAttachThreads = func(_ uint32, _ uint32, attach bool) error {
		if attach {
			current.setError(cause)
		} else {
			detached = true
		}
		return nil
	}
	_, err := current.setForegroundWithThreadAttach(1, 2)
	if !detached || !errors.Is(err, cause) {
		t.Fatalf("detach=%v err=%v", detached, err)
	}
}
