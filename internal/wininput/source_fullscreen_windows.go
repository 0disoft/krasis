//go:build windows

package wininput

import (
	"errors"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type fullscreenRect struct{ Left, Top, Right, Bottom int32 }
type fullscreenMonitorInfo struct {
	Size          uint32
	Monitor, Work fullscreenRect
	Flags         uint32
}

var (
	fullscreenGetClientRect     = user32.NewProc("GetClientRect")
	fullscreenClientToScreen    = user32.NewProc("ClientToScreen")
	fullscreenMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	fullscreenGetMonitorInfo    = user32.NewProc("GetMonitorInfoW")
	fullscreenIsIconic          = user32.NewProc("IsIconic")
	sourceCaptureIsFullscreen   = sourceWindowFullscreen
	sourceCaptureMarkFullscreen = requestSourceCaptureFullscreen
	sourceCaptureNormalHint     = setSourceCaptureNormalHint
)

var sourceNormalHintName, _ = syscall.UTF16PtrFromString("NonRudeHWND")
var sourceSetNormalHint = user32.NewProc("SetPropW")
var sourceRemoveNormalHint = user32.NewProc("RemovePropW")
var sourceGetNormalHint = user32.NewProc("GetPropW")

// Only set the Shell hint on our lifetime-registered HWND. This gives an ordinary
// capture an explicit non-fullscreen policy even when COM clearing is delayed.
func setSourceCaptureNormalHint(window uintptr, normal bool) bool {
	value, owned := sourcePresentationWindows.Load(window)
	if !owned {
		return false
	}
	s := value.(*sourcePresentationState)
	s.mu.Lock()
	live := !s.retired
	s.mu.Unlock()
	if !live {
		return false
	}
	if normal {
		ok, _, _ := sourceSetNormalHint.Call(window, uintptr(unsafe.Pointer(sourceNormalHintName)), 1)
		return ok != 0
	}
	// Absence is already the desired result; only we own this property.
	sourceRemoveNormalHint.Call(window, uintptr(unsafe.Pointer(sourceNormalHintName)))
	valueAfter, _, _ := sourceGetNormalHint.Call(window, uintptr(unsafe.Pointer(sourceNormalHintName)))
	return valueAfter == 0
}

var sourcePresentationWindows sync.Map // Owned HWND -> lifetime-specific state.
var sourcePresentationOnce sync.Once
var sourceShellPresentation *sourcePresentationBroker

func requestSourceCaptureFullscreen(window uintptr, fullscreen bool) bool {
	value, owned := sourcePresentationWindows.Load(window)
	if !owned {
		return false
	}
	sourcePresentationOnce.Do(func() { sourceShellPresentation = newSourcePresentationBroker(markSourceCaptureFullscreen) })
	return sourceShellPresentation.request(value.(*sourcePresentationState), fullscreen, sourcePresentationWait)
}

// Inspect geometry only: no titles, browser identity, URLs, or screen contents.
// Client bounds distinguish a fullscreen surface from an ordinary maximized app.
func sourceWindowFullscreen(window uintptr) bool {
	if window == 0 {
		return false
	}
	if iconic, _, _ := fullscreenIsIconic.Call(window); iconic != 0 {
		return false
	}
	if visible, _, _ := user32.NewProc("IsWindowVisible").Call(window); visible == 0 {
		return false
	}
	monitor, _, _ := fullscreenMonitorFromWindow.Call(window, 0)
	if monitor == 0 {
		return false
	}
	info := fullscreenMonitorInfo{}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := fullscreenGetMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return false
	}
	var client fullscreenRect
	if ok, _, _ := fullscreenGetClientRect.Call(window, uintptr(unsafe.Pointer(&client))); ok == 0 {
		return false
	}
	origin := point{X: client.Left, Y: client.Top}
	if ok, _, _ := fullscreenClientToScreen.Call(window, uintptr(unsafe.Pointer(&origin))); ok == 0 {
		return false
	}
	return clientCoversFullscreenMonitor(client, origin, info.Monitor)
}

func clientCoversFullscreenMonitor(client fullscreenRect, origin point, monitor fullscreenRect) bool {
	width, height := int64(client.Right)-int64(client.Left), int64(client.Bottom)-int64(client.Top)
	return width > 0 && height > 0 && monitor.Right > monitor.Left && monitor.Bottom > monitor.Top &&
		origin.X <= monitor.Left && origin.Y <= monitor.Top &&
		int64(origin.X)+width >= int64(monitor.Right) && int64(origin.Y)+height >= int64(monitor.Bottom)
}

// ITaskbarList2 follows IUnknown's three methods and ITaskbarList's five methods.
type sourceTaskbarVTable struct {
	QueryInterface, AddRef, Release                      uintptr
	HrInit, AddTab, DeleteTab, ActivateTab, SetActiveAlt uintptr
	MarkFullscreenWindow                                 uintptr
}
type sourceTaskbarList struct{ VTable *sourceTaskbarVTable }

var (
	sourceTaskbarCLSID  = windows.GUID{Data1: 0x56fdf344, Data2: 0xfd6d, Data3: 0x11d0, Data4: [8]byte{0x95, 0x8a, 0x00, 0x60, 0x97, 0xc9, 0xa0, 0x90}}
	sourceTaskbarIID    = windows.GUID{Data1: 0x602d4995, Data2: 0xb13a, Data3: 0x429b, Data4: [8]byte{0xa6, 0x6e, 0x19, 0x35, 0xe4, 0x4f, 0x43, 0x17}}
	sourceTaskbarCreate = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
)

// Called only by the serialized Shell worker. Mark our own window, never
// Explorer or the browser. Shell failures are visual degradation, not input loss.
func markSourceCaptureFullscreen(window uintptr, fullscreen bool) bool {
	err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
	if err != nil && !errors.Is(err, syscall.Errno(1)) {
		return false
	}
	defer windows.CoUninitialize()
	var taskbar *sourceTaskbarList
	hr, _, _ := sourceTaskbarCreate.Call(uintptr(unsafe.Pointer(&sourceTaskbarCLSID)), 0, 1,
		uintptr(unsafe.Pointer(&sourceTaskbarIID)), uintptr(unsafe.Pointer(&taskbar)))
	if int32(hr) < 0 || taskbar == nil {
		return false
	}
	defer syscall.SyscallN(taskbar.VTable.Release, uintptr(unsafe.Pointer(taskbar)))
	if hr, _, _ = syscall.SyscallN(taskbar.VTable.HrInit, uintptr(unsafe.Pointer(taskbar))); int32(hr) < 0 {
		return false
	}
	flag := uintptr(0)
	if fullscreen {
		flag = 1
	}
	hr, _, _ = syscall.SyscallN(taskbar.VTable.MarkFullscreenWindow, uintptr(unsafe.Pointer(taskbar)), window, flag)
	return int32(hr) >= 0
}

func (current *sourceWheelCaptureRun) inheritFullscreenPresentation() {
	if current.loginContext != nil {
		return
	} // No user Shell/COM broker before login.
	if current.fullscreenAttempted && sourceCaptureMarkFullscreen(current.window, false) {
		current.fullscreenMarked = false
		current.fullscreenAttempted = false
	}
	started := time.Now()
	fullscreen := sourceCaptureIsFullscreen(current.savedFocus)
	if current.presentationReport != nil {
		result := "normal"
		if fullscreen {
			result = "fullscreen"
		}
		current.presentationReport("geometry", result, time.Since(started))
	}
	current.setNormalPresentationHint(!fullscreen)
	if fullscreen {
		current.fullscreenAttempted = true // A timeout may still apply later; always request clearing.
		current.fullscreenMarked = sourceCaptureMarkFullscreen(current.window, true)
	}
}

func (current *sourceWheelCaptureRun) releaseFullscreenPresentation() {
	if current.loginContext != nil {
		return
	}
	if current.fullscreenAttempted {
		current.setNormalPresentationHint(true)
	}
	if current.fullscreenAttempted && sourceCaptureMarkFullscreen(current.window, false) {
		current.fullscreenMarked = false
		current.fullscreenAttempted = false
	}
}

func (current *sourceWheelCaptureRun) setNormalPresentationHint(normal bool) {
	if current.loginContext != nil {
		return
	}
	started := time.Now()
	accepted := sourceCaptureNormalHint(current.window, normal)
	if current.presentationReport != nil {
		result := "failed"
		if accepted {
			result = "accepted"
		}
		current.presentationReport("hint", result, time.Since(started))
	}
}
