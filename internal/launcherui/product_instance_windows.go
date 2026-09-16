//go:build windows

package launcherui

import (
	"context"
	"io"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/agentinstance"
)

const productShowExisting = wmUIEvent + 4

func acquireProductWindow() (io.Closer, error) {
	return acquireProductWindowForUpdate(false)
}

func acquireProductWindowForUpdate(wait bool) (io.Closer, error) {
	if wait {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return claimProductWindow(ctx, func() (io.Closer, error) { return agentinstance.AcquireControl() }, func() bool { return false })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return claimProductWindow(ctx, func() (io.Closer, error) {
		return agentinstance.AcquireControl()
	}, showExistingProductWindow)
}

func showExistingProductWindow() bool {
	window, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(utf16(productWindowClass))), 0)
	if window == 0 {
		return false
	}
	// Async restore cannot hang this process on an unresponsive older GUI.
	// Foreground activation is best effort under Windows focus restrictions.
	user32.NewProc("ShowWindowAsync").Call(window, 9) // SW_RESTORE
	user32.NewProc("SetForegroundWindow").Call(window)
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
	if pid != 0 {
		user32.NewProc("AllowSetForegroundWindow").Call(uintptr(pid))
	}
	// New GUIs restore and activate on their own UI thread. Older GUIs
	// ignore this payload-free message and use the async fallback above.
	procPostMessage.Call(window, productShowExisting, 0, 0)
	return true
}
