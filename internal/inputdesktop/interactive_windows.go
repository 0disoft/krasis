//go:build windows

package inputdesktop

import (
	"context"
	"errors"
	"runtime"
)

// RunInteractive is for the installed input-only SYSTEM/UIAccess worker. Its
// authenticated service/agent channel and lease remain independently checked.
// This thread creates no windows or hooks, so it can retire its old desktop and
// follow Default <-> Winlogon without transferring input ownership or replaying
// an event. The ordinary Run and fixed login capture mode retain their policy.
func RunInteractive(ctx context.Context, authorize func() error, callback func(*Scope) error) error {
	if ctx == nil || authorize == nil || callback == nil {
		return ErrInvalid
	}
	checked := func() error {
		if err := authorize(); err != nil {
			return err
		}
		return nil
	}
	open := func() (binding, error) {
		if err := checked(); err != nil {
			return nil, err
		}
		if !privilegedLoginToken() {
			return nil, ErrUnavailable
		}
		original, _, callErr := getThread.Call(currentThreadID())
		if original == 0 {
			return nil, errors.Join(ErrUnavailable, callErr)
		}
		handle, _, callErr := openInput.Call(0, 0, uintptr(AccessMask))
		if handle == 0 {
			return nil, desktopPreparing(errors.Join(ErrUnavailable, callErr))
		}
		name, err := readDesktopName(handle)
		if err != nil {
			return nil, desktopPreparing(errors.Join(err, closeUnbound(handle)))
		}
		var target desktopTarget
		switch name {
		case "Default":
			target = defaultTarget
		case "Winlogon":
			target = winlogonTarget
		default:
			err = errors.Join(err, ErrUnavailable)
		}
		if err != nil {
			return nil, errors.Join(err, closeUnbound(handle))
		}
		desktop := &desktopBinding{target: target, original: original, owned: handle}
		if err := desktop.verifyName(); err != nil {
			return nil, errors.Join(err, closeUnbound(handle))
		}
		if err := desktop.active(); err != nil {
			return nil, desktopPreparing(errors.Join(err, closeUnbound(handle)))
		}
		if err := checked(); err != nil {
			return nil, errors.Join(err, closeUnbound(handle))
		}
		ok, _, callErr := setThread.Call(handle)
		if ok == 0 {
			return nil, errors.Join(ErrUnavailable, callErr, closeUnbound(handle))
		}
		return desktop, nil
	}
	return run(ctx, callback, func() (binding, error) {
		return &interactiveBinding{open: open, authorize: checked}, nil
	}, currentThreadID, runtime.LockOSThread, runtime.UnlockOSThread)
}
