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
	target         desktopTarget
	original       uintptr // Borrowed: never close the GetThreadDesktop result.
	owned          uintptr
	verifiedHandle uintptr // The selected fixed name belongs to this exact handle.
}

type desktopTarget uint8

const (
	defaultTarget desktopTarget = iota
	winlogonTarget
)

func desktopName(target desktopTarget) (string, error) {
	switch target {
	case defaultTarget:
		return "Default", nil
	case winlogonTarget:
		return "Winlogon", nil
	default:
		return "", ErrUnavailable
	}
}

func currentThreadID() uintptr {
	id, _, _ := getThreadID.Call()
	return id
}

func openDesktop() (binding, error) {
	return openTargetDesktop(defaultTarget)
}

func openTargetDesktop(target desktopTarget) (binding, error) {
	if _, err := desktopName(target); err != nil {
		return nil, err
	}
	original, _, callErr := getThread.Call(currentThreadID())
	if original == 0 {
		return nil, errors.Join(ErrUnavailable, operationFailure("desktop-current", callErr))
	}
	handle, _, callErr := openInput.Call(0, 0, uintptr(AccessMask))
	if handle == 0 {
		return nil, errors.Join(ErrUnavailable, operationFailure("desktop-open", callErr))
	}
	desktop := &desktopBinding{target: target, original: original, owned: handle}
	if err := desktop.verifyName(); err != nil {
		return nil, errors.Join(ErrUnavailable, err, closeUnbound(handle))
	}
	if err := desktop.active(); err != nil {
		return nil, errors.Join(ErrUnavailable, err, closeUnbound(handle))
	}
	ok, _, callErr := setThread.Call(handle)
	if ok == 0 {
		return nil, errors.Join(ErrUnavailable, operationFailure("desktop-bind", callErr), closeUnbound(handle))
	}
	return desktop, nil
}

var readDesktopName = func(handle uintptr) (string, error) {
	var name [128]uint16
	var needed uint32
	ok, _, callErr := getInformation.Call(handle, 2, uintptr(unsafe.Pointer(&name[0])),
		uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed))) // UOI_NAME
	if ok == 0 {
		return "", errors.Join(ErrInactive, operationFailure("desktop-name", callErr))
	}
	return syscall.UTF16ToString(name[:]), nil
}

var readDesktopInput = func(handle uintptr) (bool, error) {
	var receiving int32
	var needed uint32
	ok, _, callErr := getInformation.Call(handle, 6, uintptr(unsafe.Pointer(&receiving)),
		unsafe.Sizeof(receiving), uintptr(unsafe.Pointer(&needed))) // UOI_IO
	if ok == 0 {
		return false, errors.Join(ErrInactive, operationFailure("desktop-active", callErr))
	}
	return receiving != 0, nil
}

// The object name is validated when a handle is acquired. It is not an input
// activity cache: active() still queries the changing UOI_IO before every work.
func (desktop *desktopBinding) verifyName() error {
	if desktop == nil || desktop.owned == 0 {
		return ErrClosed
	}
	desktop.verifiedHandle = 0
	expected, targetErr := desktopName(desktop.target)
	name, err := readDesktopName(desktop.owned)
	if err != nil || targetErr != nil || name != expected {
		return errors.Join(ErrInactive, err, targetErr)
	}
	desktop.verifiedHandle = desktop.owned
	return nil
}

func (desktop *desktopBinding) active() error {
	if desktop == nil || desktop.owned == 0 {
		return ErrClosed
	}
	if desktop.verifiedHandle != desktop.owned {
		return ErrInactive
	}
	receiving, err := readDesktopInput(desktop.owned)
	if err != nil {
		return errors.Join(ErrInactive, err)
	}
	if !receiving {
		return ErrInactive
	}
	return nil
}

func (desktop *desktopBinding) close() error {
	if desktop == nil || desktop.owned == 0 {
		return ErrClosed
	}
	desktop.verifiedHandle = 0 // A closing/failed restoration cannot retain proof.
	ok, _, callErr := setThread.Call(desktop.original)
	if ok == 0 {
		// The worker must exit. Do not close a still-attached desktop or return
		// the contaminated thread to the runtime pool.
		return errors.Join(ErrCleanup, operationFailure("desktop-restore", callErr))
	}
	handle := desktop.owned
	desktop.owned = 0
	return closeUnbound(handle)
}

func closeUnbound(handle uintptr) error {
	ok, _, callErr := closeHandle.Call(handle)
	if ok == 0 {
		return errors.Join(ErrCleanup, operationFailure("desktop-close", callErr))
	}
	return nil
}
