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

// The installed source-only child may bind Default or Winlogon. Each native
// owner retires before a new desktop is selected; no caller chooses its name.
func NewProtectedSourceHook(authorize func() error) (*SourceHook, error) {
	hook, err := NewLoginSourceHook(authorize)
	if err == nil {
		hook.interactiveSource = true
	}
	return hook, err
}

// Admission/native pressed-state queries run on the same authenticated desktop
// policy as capture. A medium process cannot use this to gain a token or scope.
func (hook *SourceHook) ExecuteSourceDesktop(ctx context.Context, callback func() error) error {
	if ctx == nil || callback == nil {
		return inputdesktop.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if hook.interactiveSource {
		// These commands create no windows or hooks: activation and cursor work
		// are posted to their existing native owners. Borrow/restore a desktop
		// rather than creating and joining a disposable capture thread per call.
		// The actual hook/window owners still use RunCapture below.
		return sourceCommandDesktop(context.Background(), hook.loginAuthorize, func(scope *inputdesktop.Scope) error {
			return scope.Do(ctx, callback)
		})
	}
	return hook.onDesktop(ctx, func(owned context.Context) error {
		if err := sourceLoginDesktopCheck(owned); err != nil {
			return err
		}
		return callback()
	})
}

var sourceCommandDesktop = inputdesktop.RunPresentation

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
	// Cancellation stops the native loop. Capture scope cleanup waits for native
	// owner retirement; fixed Winlogon scopes still restore their borrowed desktop.
	bind := inputdesktop.RunWinlogon
	if hook.interactiveSource {
		bind = inputdesktop.RunCapture
	}
	return bind(context.Background(), hook.loginAuthorize, func(scope *inputdesktop.Scope) error {
		if !hook.interactiveSource {
			// Fixed Winlogon mode still restores its desktop before returning.
			// Retire its native message queue rather than pooling the thread.
			runtime.LockOSThread()
		}
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
