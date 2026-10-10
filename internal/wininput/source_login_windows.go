//go:build windows

package wininput

import (
	"context"
	"errors"
	"runtime"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

type sourceLoginDesktopKey struct{}

// NewLoginSourceHook is a separate, fixed Winlogon mode for the authenticated
// installed service child. Every native owner binds before creating a window or
// hook. It does not change ordinary SourceHook's desktop or elevate its caller.
func NewLoginSourceHook(authorize func() error) (*SourceHook, error) {
	if authorize == nil {
		return nil, inputdesktop.ErrInvalid
	}
	hook := NewSourceHook()
	hook.loginAuthorize = authorize
	return hook, nil
}

func (hook *SourceHook) onDesktop(ctx context.Context, run func(context.Context) error) error {
	if hook.loginAuthorize == nil {
		return run(ctx)
	}
	if ctx == nil || run == nil {
		return inputdesktop.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Cancellation stops the native loop, while the scope survives until the
	// loop has unregistered its hooks/windows and restored its owned desktop.
	return inputdesktop.RunWinlogon(context.Background(), hook.loginAuthorize, func(scope *inputdesktop.Scope) error {
		// RunWinlogon's own lock is released only after desktop restoration.
		// Retain an extra lock so this goroutine's exit retires the native
		// capture thread rather than pooling its old Win32 message queue.
		runtime.LockOSThread()
		owned := context.WithValue(ctx, sourceLoginDesktopKey{}, scope)
		return run(owned)
	})
}

func sourceLoginScope(ctx context.Context) *inputdesktop.Scope {
	if ctx == nil {
		return nil
	}
	scope, _ := ctx.Value(sourceLoginDesktopKey{}).(*inputdesktop.Scope)
	return scope
}

func sourceLoginDesktopCheck(ctx context.Context) error {
	if scope := sourceLoginScope(ctx); scope != nil {
		return scope.Do(ctx, func() error { return nil })
	}
	return nil
}

// An inactive Winlogon desktop needs only cleanup of our owned resources. Do
// not restore another foreground window after the user has switched desktops.
func (current *sourceWheelCaptureRun) retireLoginCapture() error {
	current.active.Store(false)
	err := current.detachForegroundThreads()
	if current.registered {
		if failure := sourceWheelCaptureRegisterWindow(current.registerProc, current.window, false); failure != nil {
			err = errors.Join(err, ErrSourceCaptureUnregisterFailed, failure)
		} else {
			current.registered = false
		}
	}
	current.forgetSavedFocus()
	return errors.Join(err, current.hideAfterFocusRestore())
}
