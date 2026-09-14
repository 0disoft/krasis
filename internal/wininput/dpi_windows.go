//go:build windows

package wininput

import (
	"errors"
	"syscall"
	"unsafe"
)

// InitializePhysicalCoordinates must run before creating windows, hooks or
// reading screen geometry. It sets the process default, not a transient Go
// goroutine's thread context. The management GUI is a separate process.
func InitializePhysicalCoordinates() error {
	shcore := syscall.NewLazyDLL("shcore.dll")
	get := shcore.NewProc("GetProcessDpiAwareness")
	v2 := user32.NewProc("SetProcessDpiAwarenessContext")
	v1 := shcore.NewProc("SetProcessDpiAwareness")
	return initializePhysicalCoordinates(func() int32 {
		if get.Find() != nil {
			return -1
		}
		var value int32
		hr, _, _ := get.Call(0, uintptr(unsafe.Pointer(&value)))
		if hr != 0 {
			return -1
		}
		return value
	}, func() {
		if v2.Find() == nil {
			v2.Call(^uintptr(3))
		} // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	}, func() {
		if v1.Find() == nil {
			v1.Call(2)
		} // PROCESS_PER_MONITOR_DPI_AWARE
	})
}

func initializePhysicalCoordinates(awareness func() int32, setV2, setV1 func()) error {
	if awareness() == 2 {
		return nil
	}
	setV2()
	if awareness() == 2 {
		return nil
	}
	// Older Windows versions may support per-monitor v1 but not v2. Never
	// fall back to system-aware or unaware geometry.
	setV1()
	if awareness() == 2 {
		return nil
	}
	return errors.New("per-monitor DPI awareness could not be established")
}
