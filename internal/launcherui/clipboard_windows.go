//go:build windows

package launcherui

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procRtlMoveMemory    = syscall.NewLazyDLL("ntdll.dll").NewProc("RtlMoveMemory")
)

func writeClipboardText(owner uintptr, value string) error {
	encoded, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	size := uintptr(len(encoded) * 2)
	handle, _, callErr := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return fmt.Errorf("allocate clipboard memory: %w", callErr)
	}
	transferred := false
	defer func() {
		if !transferred {
			procGlobalFree.Call(handle)
		}
	}()
	pointer, _, callErr := procGlobalLock.Call(handle)
	if pointer == 0 {
		return fmt.Errorf("lock clipboard memory: %w", callErr)
	}
	procRtlMoveMemory.Call(pointer, uintptr(unsafe.Pointer(&encoded[0])), size)
	runtime.KeepAlive(encoded)
	procGlobalUnlock.Call(handle)
	if result, _, callErr := procOpenClipboard.Call(owner); result == 0 {
		return fmt.Errorf("open clipboard: %w", callErr)
	}
	defer procCloseClipboard.Call()
	if result, _, callErr := procEmptyClipboard.Call(); result == 0 {
		return fmt.Errorf("empty clipboard: %w", callErr)
	}
	if result, _, callErr := procSetClipboardData.Call(cfUnicodeText, handle); result == 0 {
		if errors.Is(callErr, syscall.Errno(0)) {
			callErr = errors.New("SetClipboardData returned zero")
		}
		return fmt.Errorf("set clipboard data: %w", callErr)
	}
	transferred = true
	return nil
}
