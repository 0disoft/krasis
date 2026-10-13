//go:build windows

package inputdesktop

import (
	"context"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

type observedRetiringThread struct {
	retiringThread
	verified bool
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
		return nil
	}, func() (retiringBinding, error) { return desktop, nil }, currentThreadID,
		func() (retiringThread, error) {
			native, err := pinRetiringThread()
			if err != nil {
				return nil, err
			}
			owner = &observedRetiringThread{retiringThread: native}
			return owner, nil
		}, runtime.LockOSThread)
	if err != nil || owner == nil || !owner.verified || thread == 0 || desktop.retired.Load() != 1 || desktop.closed.Load() != 0 {
		t.Fatal("native thread exit was not confirmed before desktop close", err)
	}
}
