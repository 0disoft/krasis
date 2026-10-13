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
	}, currentThreadID, pinRetiringThread, runtime.LockOSThread)
}

type nativeRetiringThread struct{ handle windows.Handle }

func pinRetiringThread() (retiringThread, error) {
	var handle windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.CurrentThread(),
		windows.CurrentProcess(), &handle, windows.SYNCHRONIZE, false, 0); err != nil {
		return nil, operationFailure("desktop-thread-exit", err)
	}
	return &nativeRetiringThread{handle: handle}, nil
}

func (t *nativeRetiringThread) join() error {
	state, err := windows.WaitForSingleObject(t.handle, 1000)
	if err != nil {
		return err
	}
	if state != windows.WAIT_OBJECT_0 {
		return errors.New("capture native thread exit unconfirmed")
	}
	return nil
}

func (t *nativeRetiringThread) close() error { return windows.CloseHandle(t.handle) }

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
