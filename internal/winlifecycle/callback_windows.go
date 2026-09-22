//go:build windows

package winlifecycle

import (
	"sync"
	"syscall"
)

// Go callback trampolines live for the entire process. Keep one trampoline and
// retire each window's owner only after DestroyWindow has finished its callbacks.
var lifecycleWindowOwners sync.Map
var lifecycleWindowCallback = syscall.NewCallback(dispatchLifecycleWindow)

func dispatchLifecycleWindow(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	if owner, ok := lifecycleWindowOwners.Load(window); ok {
		monitor := owner.(*Monitor)
		if event, ok := eventFromMessage(message, wParam); ok {
			monitor.publish(event)
		}
		switch message {
		case wmQueryEndSession:
			return 1
		case wmClose, wmLifecycleStop:
			procPostQuitMessage.Call(0)
			return 0
		case wmDestroy:
			monitor.windowDestroyed = true
			if !monitor.tearingDown {
				procPostQuitMessage.Call(0)
			}
			return 0
		}
	}
	result, _, _ := procDefWindowProcW.Call(window, uintptr(message), wParam, lParam)
	return result
}
