//go:build windows

package wininput

import (
	"errors"
	"testing"
)

func TestFailedUnregisterRetainsObligationForNextCleanup(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	calls := 0
	sourceWheelCaptureRegisterWindow = func(uintptr, uintptr, bool) error {
		calls++
		if calls == 1 {
			return errors.New("unregister denied")
		}
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10, registered: true}
	if err := current.deactivateWindow(); !errors.Is(err, ErrSourceCaptureUnregisterFailed) || !current.registered {
		t.Fatalf("lost registration obligation: %v", err)
	}
	if err := current.deactivateWindow(); err != nil || current.registered || calls != 2 {
		t.Fatalf("cleanup retry: %v registered=%v calls=%d", err, current.registered, calls)
	}
	if err := current.deactivateWindow(); err != nil || calls != 2 {
		t.Fatal("completed cleanup was not idempotent")
	}
}

func TestDetachRetryUsesExactOwnerPairAndCannotReattach(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	sourceWheelCaptureCurrentThread = func() uint32 { return 11 }
	sourceWheelCaptureWindowThread = func(uintptr) (uint32, error) { return 22, nil }
	sourceWheelCaptureGetForeground = func() uintptr { return 10 }
	sourceWheelCaptureSetForeground = func(uintptr) error { return nil }
	detachCalls, attachCalls := 0, 0
	sourceWheelCaptureAttachThreads = func(source, target uint32, attach bool) error {
		if source != 11 || target != 22 {
			t.Fatalf("changed native pair: %d %d", source, target)
		}
		if attach {
			attachCalls++
			return nil
		}
		detachCalls++
		if detachCalls <= 2 {
			return errors.New("detach denied")
		}
		return nil
	}
	current := &sourceWheelCaptureRun{window: 10}
	if result, _ := current.setForegroundWithThreadAttach(10, 20); result != foregroundThreadFallbackDetachFailed {
		t.Fatal("detach failure ignored")
	}
	if current.attachedSource != 11 || current.attachedPeer != 22 {
		t.Fatal("failed attachment forgotten")
	}
	_, _ = current.setForegroundWithThreadAttach(10, 30)
	if attachCalls != 1 {
		t.Fatal("new attachment allowed before cleanup")
	}
	if err := current.deactivateWindow(); !errors.Is(err, ErrSourceCaptureThreadDetachFailed) || current.attachedSource == 0 {
		t.Fatalf("lost failed detach: %v", err)
	}
	if err := current.deactivateWindow(); err != nil || current.attachedSource != 0 || detachCalls != 3 {
		t.Fatalf("detach retry failed: %v", err)
	}
}
