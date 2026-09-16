//go:build windows

package wininput

import (
	"sync"
	"syscall"
)

type sourceHookCallback func(int, uintptr, uintptr) uintptr
type sourceWindowCallback func(uintptr, uint32, uintptr, uintptr) uintptr

type sourceHookCallbacks struct {
	keyboard sourceHookCallback
	mouse    sourceHookCallback
}

// NewCallback allocates process-lifetime trampolines. Keep exactly one per
// signature/role; only ordinary, collectable Go closures vary by native owner.
var sourceHookOwners sync.Map   // OS thread ID -> *sourceHookCallbacks
var sourceWindowOwners sync.Map // HWND -> sourceWindowCallback

var sourceKeyboardCallback = syscall.NewCallback(func(code int, w, l uintptr) uintptr {
	thread, _, _ := procGetCurrentThreadID.Call()
	if owner, ok := sourceHookOwners.Load(thread); ok {
		return owner.(*sourceHookCallbacks).keyboard(code, w, l)
	}
	return callNextHook(code, w, l)
})

var sourceMouseCallback = syscall.NewCallback(func(code int, w, l uintptr) uintptr {
	thread, _, _ := procGetCurrentThreadID.Call()
	if owner, ok := sourceHookOwners.Load(thread); ok {
		return owner.(*sourceHookCallbacks).mouse(code, w, l)
	}
	return callNextHook(code, w, l)
})

var sourceCaptureCallback = syscall.NewCallback(func(window uintptr, message uint32, w, l uintptr) uintptr {
	if owner, ok := sourceWindowOwners.Load(window); ok {
		return owner.(sourceWindowCallback)(window, message, w, l)
	}
	result, _, _ := procDefWindowProcWTouchpad.Call(window, uintptr(message), w, l)
	return result
})
