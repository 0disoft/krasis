//go:build windows

package inputdesktop

import (
	"context"
	"errors"
	"runtime"

	"golang.org/x/sys/windows"
)

// RunCapture uses the same fixed Default/Winlogon selection and privileged token
// gate as RunPresentation. Only capture's disposable native threads use it;
// ordinary input/presentation scopes keep their restoration policy.
func RunCapture(ctx context.Context, authorize func() error, callback func(*Scope) error) error {
	if ctx == nil || authorize == nil || callback == nil {
		return ErrInvalid
	}
	return runRetiring(ctx, callback, func() (retiringBinding, error) {
		b, err := openInteractiveDesktop(authorize)
		if err != nil {
			return nil, err
		}
		return b.(*desktopBinding), nil
	}, currentThreadID, pinRetiringThread, startRetiringThread)
}

// Package initialization runs on Go's startup thread. Unlike other locked
// threads, runtime.mexit parks m0 forever rather than terminating it. Capture
// must never attach a desktop to that thread and wait for its kernel exit.
var captureStartupThread = currentThreadID()

func startRetiringThread(run func()) {
	go func() {
		runtime.LockOSThread()
		if currentThreadID() != captureStartupThread {
			run()
			return // A disposable native owner exits with its lock held.
		}
		// Hold the startup thread until the replacement has locked another OS
		// thread. This guard acquires no desktop, window, hook or input state.
		locked := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			close(locked)
			run()
		}()
		<-locked
		runtime.UnlockOSThread()
	}()
}

type nativeRetiringThread struct {
	handle  windows.Handle
	startup bool
	wait    string
}

func pinRetiringThread() (retiringThread, error) {
	var handle windows.Handle
	startup := currentThreadID() == captureStartupThread
	if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.CurrentThread(),
		windows.CurrentProcess(), &handle, windows.SYNCHRONIZE, false, 0); err != nil {
		return nil, captureThreadFailure("pin", startup, "", err)
	}
	return &nativeRetiringThread{handle: handle, startup: startup}, nil
}

func (t *nativeRetiringThread) join() error {
	state, err := windows.WaitForSingleObject(t.handle, 1000)
	t.wait, err = retiringWaitResult(state, err)
	if err != nil {
		return captureThreadFailure("wait", t.startup, t.wait, err)
	}
	return nil
}

func retiringWaitResult(state uint32, err error) (string, error) {
	if err != nil {
		return "failed", err
	}
	if state != windows.WAIT_OBJECT_0 {
		if state == uint32(windows.WAIT_TIMEOUT) {
			return "timeout", windows.ERROR_TIMEOUT
		}
		return "unexpected", errors.New("capture native thread exit unconfirmed")
	}
	return "signaled", nil
}

func (t *nativeRetiringThread) close() error {
	return captureThreadFailure("close", t.startup, t.wait, windows.CloseHandle(t.handle))
}

// The caller has proved the exact bound OS thread terminated. CloseDesktop
// cannot race its windows or attachment; the original desktop was only borrowed.
func (d *desktopBinding) closeRetired() error {
	if d == nil || d.owned == 0 {
		return ErrClosed
	}
	d.verifiedHandle = 0
	handle := d.owned
	d.owned = 0
	return closeUnbound(handle)
}
