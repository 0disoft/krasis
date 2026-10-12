//go:build windows

package wininput

import (
	"context"
	"errors"
	"runtime"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

// RunProtectedReturnCursor is a passive, fixed Winlogon window owned by an
// independently authenticated SYSTEM/UIAccess child. It has no input port,
// capture, foreground activation or global cursor/display-count changes.
func RunProtectedReturnCursor(ctx context.Context, authorize func() error, x, y int32, ready func() error) error {
	if ctx == nil || authorize == nil || ready == nil || ctx.Err() != nil {
		return inputdesktop.ErrInvalid
	}
	return inputdesktop.RunWinlogon(context.Background(), authorize, func(scope *inputdesktop.Scope) (result error) {
		// Discard this window/hook thread after desktop restoration, matching
		// login capture owners; its old Win32 queue never enters Go's pool.
		runtime.LockOSThread()
		owned := context.WithValue(ctx, sourceLoginDesktopKey{}, scope)
		shield := &returnCursorShield{loginContext: owned, position: point{X: x, Y: y}}
		shield.open()
		defer func() {
			if err := shield.close(); err != nil {
				result = errors.Join(result, inputdesktop.ErrCleanup, err)
			}
		}()
		if shield.window.Load() == 0 {
			return errors.New("protected return cursor window unavailable")
		}
		local := false
		closeHooks, err := passiveReturnHooks(shield, &local)
		if err != nil {
			return err
		}
		defer func() {
			if err := closeHooks(); err != nil {
				result = errors.Join(result, inputdesktop.ErrCleanup, err)
			}
		}()
		return runProtectedReturnCursorOwner(ctx, authorize, func() error { return scope.Do(ctx, func() error { return nil }) }, shield, ready, func() bool { return local })
	})
}

var protectedReturnMessages = func() {
	for i := 0; i < 16; i++ {
		var msg message
		if ok, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1); ok == 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func runProtectedReturnCursorOwner(ctx context.Context, authorize, active func() error, shield *returnCursorShield, ready func() error, local func() bool) error {
	if err := authorize(); err != nil {
		return err
	}
	if err := active(); err != nil {
		return err
	}
	position, err := returnCursorPosition()
	if err != nil {
		return err
	}
	// A local move/press while the child started invalidates the snapshot. Do
	// not hide the user's new cursor or retain a stale request.
	if position != shield.position || sourceAnyInputPressed() || local() {
		return ready()
	}
	shield.show(shield.generation.Load())
	if err := shield.failureResult(); err != nil {
		return err
	}
	if err := ready(); err != nil {
		return err
	}
	timer := time.NewTicker(20 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil // Owner cleanup is still checked by caller.
		case <-timer.C:
		}
		if err := authorize(); err != nil {
			return err
		}
		if err := active(); err != nil {
			if passiveDesktopInactive(err) {
				return nil
			}
			return err
		}
		position, err := returnCursorPosition()
		if err != nil {
			return err
		}
		if position != shield.position || sourceAnyInputPressed() || local() {
			return nil
		}
		protectedReturnMessages()
	}
}

func passiveDesktopInactive(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !passiveDesktopInactive(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return passiveDesktopInactive(wrapped.Unwrap())
	}
	return err == inputdesktop.ErrInactive
}

// These hooks restore the passive window before Windows routes the first local
// click/key. They never inspect key contents, suppress, forward or inject input.
func passiveReturnHooks(shield *returnCursorShield, local *bool) (func() error, error) {
	thread, _, _ := procGetCurrentThreadID.Call()
	callback := func(code int, wParam, lParam uintptr) uintptr {
		if code == hcAction {
			*local = true
			_ = shield.beforeInput()
		}
		return callNextHook(code, wParam, lParam)
	}
	owner := &sourceHookCallbacks{keyboard: callback, mouse: callback}
	if _, loaded := sourceHookOwners.LoadOrStore(thread, owner); loaded {
		return nil, errors.New("passive cursor hook owner conflict")
	}
	module, _, _ := procGetModuleHandleW.Call(0)
	if module == 0 {
		sourceHookOwners.CompareAndDelete(thread, owner)
		return nil, errors.New("passive cursor module unavailable")
	}
	keyboard, _, _ := procSetWindowsHookExW.Call(whKeyboardLL, sourceKeyboardCallback, module, 0)
	mouse, _, _ := procSetWindowsHookExW.Call(whMouseLL, sourceMouseCallback, module, 0)
	closeHooks := func() error {
		var failure error
		for _, handle := range []uintptr{keyboard, mouse} {
			if handle != 0 {
				if ok, _, err := procUnhookWindowsHookEx.Call(handle); ok == 0 {
					failure = errors.Join(failure, err, inputdesktop.ErrCleanup)
				}
			}
		}
		if failure == nil {
			sourceHookOwners.CompareAndDelete(thread, owner)
		}
		return failure
	}
	if keyboard == 0 || mouse == 0 {
		return nil, errors.Join(errors.New("passive cursor restoration hook unavailable"), closeHooks())
	}
	return closeHooks, nil
}
