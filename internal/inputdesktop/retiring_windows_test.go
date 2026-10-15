//go:build windows

package inputdesktop

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

type observedRetiringThread struct {
	retiringThread
	verified bool
}

func TestNativeRetiringWaitClassificationPreservesWin32AndNeverInfersExit(t *testing.T) {
	for _, tc := range []struct {
		state uint32
		cause error
		want  string
	}{
		{state: windows.WAIT_OBJECT_0, want: "signaled"},
		{state: uint32(windows.WAIT_TIMEOUT), want: "timeout"},
		{state: windows.WAIT_FAILED, cause: windows.ERROR_INVALID_HANDLE, want: "failed"},
		{state: windows.WAIT_OBJECT_0, cause: windows.ERROR_ACCESS_DENIED, want: "failed"},
		{state: 12345, want: "unexpected"},
	} {
		got, err := retiringWaitResult(tc.state, tc.cause)
		if got != tc.want || (err == nil) != (got == "signaled") || tc.cause != nil && !errors.Is(err, tc.cause) {
			t.Fatal("thread wait result changed its proof", got, err)
		}
	}
	// Read-only invalid-handle operations verify native errors cross the actual
	// join/close methods without creating a window, hook or input operation.
	owner := &nativeRetiringThread{}
	if err := owner.join(); !errors.Is(err, windows.ERROR_INVALID_HANDLE) || FailureOperation(err) != "desktop-thread-wait-other-failed" {
		t.Fatal("native wait failure lost", err)
	}
	if err := owner.close(); !errors.Is(err, windows.ERROR_INVALID_HANDLE) || FailureOperation(err) != "desktop-thread-close-other-failed" {
		t.Fatal("native close failure lost", err)
	}
}

func (t *observedRetiringThread) join() error {
	err := t.retiringThread.join()
	if err == nil {
		t.verified = true
	}
	return err
}

func TestCaptureRetirementWaitsForRealWindowsThreadWithoutInput(t *testing.T) {
	// Uses a real pinned native thread but no desktop/window/hook/input mutation.
	desktop := &testRetiringBinding{}
	var owner *observedRetiringThread
	var thread uint32
	err := runRetiring(context.Background(), func(scope *Scope) error {
		thread = windows.GetCurrentThreadId()
		if uintptr(thread) != scope.owner {
			t.Error("capture scope changed native thread")
		}
		if uintptr(thread) == captureStartupThread {
			t.Error("capture acquired Go's non-terminating startup thread")
		}
		return nil
	}, func() (retiringBinding, error) { return desktop, nil }, currentThreadID,
		func() (retiringThread, error) {
			native, err := pinRetiringThread()
			if err != nil {
				return nil, err
			}
			owner = &observedRetiringThread{retiringThread: native}
			return owner, nil
		}, startRetiringThread)
	if err != nil || owner == nil || !owner.verified || thread == 0 || desktop.retired.Load() != 1 || desktop.closed.Load() != 0 {
		t.Fatal("native thread exit was not confirmed before desktop close", err)
	}
}
