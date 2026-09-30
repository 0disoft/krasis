//go:build windows

package inputdesktop

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	desktopUser32  = syscall.NewLazyDLL("user32.dll")
	desktopKernel  = syscall.NewLazyDLL("kernel32.dll")
	openInput      = desktopUser32.NewProc("OpenInputDesktop")
	getThread      = desktopUser32.NewProc("GetThreadDesktop")
	setThread      = desktopUser32.NewProc("SetThreadDesktop")
	closeHandle    = desktopUser32.NewProc("CloseDesktop")
	getInformation = desktopUser32.NewProc("GetUserObjectInformationW")
	getThreadID    = desktopKernel.NewProc("GetCurrentThreadId")
)

type desktopBinding struct {
	original uintptr // Borrowed: never close the GetThreadDesktop result.
	owned    uintptr
}

func currentThreadID() uintptr {
	id, _, _ := getThreadID.Call()
	return id
}

func openDesktop() (binding, error) {
	original, _, callErr := getThread.Call(currentThreadID())
	if original == 0 {
		return nil, errors.Join(ErrUnavailable, callErr)
	}
	handle, _, callErr := openInput.Call(0, 0, uintptr(AccessMask))
	if handle == 0 {
		return nil, errors.Join(ErrUnavailable, callErr)
	}
	desktop := &desktopBinding{original: original, owned: handle}
	if err := desktop.active(); err != nil {
		return nil, errors.Join(ErrUnavailable, err, closeUnbound(handle))
	}
	ok, _, callErr := setThread.Call(handle)
	if ok == 0 {
		return nil, errors.Join(ErrUnavailable, callErr, closeUnbound(handle))
	}
	return desktop, nil
}

func (desktop *desktopBinding) active() error {
	if desktop == nil || desktop.owned == 0 {
		return ErrClosed
	}
	var name [128]uint16
	var needed uint32
	ok, _, callErr := getInformation.Call(desktop.owned, 2, uintptr(unsafe.Pointer(&name[0])),
		uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed))) // UOI_NAME
	if ok == 0 {
		return errors.Join(ErrInactive, callErr)
	}
	if syscall.UTF16ToString(name[:]) != "Default" {
		return ErrInactive
	}
	var receiving int32
	ok, _, callErr = getInformation.Call(desktop.owned, 6, uintptr(unsafe.Pointer(&receiving)),
		unsafe.Sizeof(receiving), uintptr(unsafe.Pointer(&needed))) // UOI_IO
	if ok == 0 {
		return errors.Join(ErrInactive, callErr)
	}
	if receiving == 0 {
		return ErrInactive
	}
	return nil
}

func (desktop *desktopBinding) close() error {
	if desktop == nil || desktop.owned == 0 {
		return ErrClosed
	}
	ok, _, callErr := setThread.Call(desktop.original)
	if ok == 0 {
		// The worker must exit. Do not close a still-attached desktop or return
		// the contaminated thread to the runtime pool.
		return errors.Join(ErrCleanup, callErr)
	}
	handle := desktop.owned
	desktop.owned = 0
	return closeUnbound(handle)
}

func closeUnbound(handle uintptr) error {
	ok, _, callErr := closeHandle.Call(handle)
	if ok == 0 {
		return errors.Join(ErrCleanup, callErr)
	}
	return nil
}
