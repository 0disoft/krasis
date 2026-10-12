//go:build windows

package wininput

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	procOpenInputDesktop         = user32.NewProc("OpenInputDesktop")
	procCloseDesktop             = user32.NewProc("CloseDesktop")
	procGetUserObjectInformation = user32.NewProc("GetUserObjectInformationW")
	procGetProtectedForeground   = user32.NewProc("GetForegroundWindow")
	procGetProtectedWindowPID    = user32.NewProc("GetWindowThreadProcessId")
	procOpenProtectedProcess     = kernel32.NewProc("OpenProcess")
	procQueryProtectedImage      = kernel32.NewProc("QueryFullProcessImageNameW")
)

// ProtectedInputActive inspects only the input desktop and foreground process
// identity. It never reads a window title, credential field, or window content.
func ProtectedInputActive() bool {
	return InspectProtectedInput() != ProtectedInputClear
}

// InspectProtectedInput classifies the same observation used by the guard.
func InspectProtectedInput() ProtectedInputReason {
	desktop := inspectInputDesktop()
	if desktop != ProtectedInputClear {
		return desktop
	}
	return protectedCredentialReason(inspectCredentialForeground())
}

func protectedCredentialReason(state credentialForegroundState) ProtectedInputReason {
	switch state {
	case credentialForegroundBroker:
		return ProtectedInputCredentialBroker
	case credentialForegroundMissing, credentialForegroundOther, credentialForegroundLockScreen:
		return ProtectedInputClear
	default:
		return ProtectedInputIdentityUnknown
	}
}

// CredentialBrokerForeground is deliberately narrower than ProtectedInputActive:
// the probe must never send input to a non-default or unidentifiable desktop.
func CredentialBrokerForeground() bool {
	return inputDesktopDefault() && inspectCredentialForeground() == credentialForegroundBroker
}

func inputDesktopDefault() bool {
	return inspectInputDesktop() == ProtectedInputClear
}

func inspectInputDesktop() ProtectedInputReason {
	desktop, _, _ := procOpenInputDesktop.Call(0, 0, 0x0001) // DESKTOP_READOBJECTS
	if desktop == 0 {
		return ProtectedInputDesktopUnavailable
	}
	var name [128]uint16
	var needed uint32
	ok, _, _ := procGetUserObjectInformation.Call(desktop, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed))) // UOI_NAME
	procCloseDesktop.Call(desktop)
	if ok == 0 {
		return ProtectedInputDesktopUnavailable
	}
	if syscall.UTF16ToString(name[:]) != "Default" {
		return ProtectedInputNonDefaultDesktop
	}
	return ProtectedInputClear
}

type credentialForegroundState uint8

const (
	credentialForegroundMissing credentialForegroundState = iota
	credentialForegroundUnknown
	credentialForegroundOther
	credentialForegroundBroker
	credentialForegroundLockScreen
)

// Only presentation is routed here. LockApp remains an ordinary capture target;
// choosing the installed cursor owner grants no new input authority.
func ProtectedReturnCursorRequired() bool {
	if inspectInputDesktop() != ProtectedInputClear {
		return true
	}
	switch inspectCredentialForeground() {
	case credentialForegroundLockScreen, credentialForegroundBroker, credentialForegroundUnknown:
		return true
	default:
		return false
	}
}

func inspectCredentialForeground() credentialForegroundState {
	window, _, _ := procGetProtectedForeground.Call()
	if window == 0 {
		return credentialForegroundMissing
	}
	var pid uint32
	procGetProtectedWindowPID.Call(window, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return credentialForegroundUnknown
	}
	process, _, _ := procOpenProtectedProcess.Call(0x1000, 0, uintptr(pid)) // PROCESS_QUERY_LIMITED_INFORMATION
	if process == 0 {
		if protectedWindowStillForeground(window) {
			return credentialForegroundUnknown
		}
		return credentialForegroundMissing
	}
	defer procCloseHandle.Call(process)
	var image [1024]uint16
	size := uint32(len(image))
	ok, _, _ := procQueryProtectedImage.Call(process, 0, uintptr(unsafe.Pointer(&image[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 || size == 0 || size > uint32(len(image)) {
		if protectedWindowStillForeground(window) {
			return credentialForegroundUnknown
		}
		return credentialForegroundMissing
	}
	if !protectedWindowStillForeground(window) {
		return credentialForegroundMissing
	}
	if isCredentialBrokerImage(syscall.UTF16ToString(image[:size])) {
		return credentialForegroundBroker
	}
	if isLockScreenImage(syscall.UTF16ToString(image[:size])) {
		return credentialForegroundLockScreen
	}
	return credentialForegroundOther
}

func credentialProbeObservation() CredentialUIProbeResult {
	if !inputDesktopDefault() {
		return CredentialUIProbeDesktopUnavailable
	}
	switch inspectCredentialForeground() {
	case credentialForegroundBroker:
		return CredentialUIProbeBrokerObserved
	case credentialForegroundUnknown:
		return CredentialUIProbeIdentityUnknown
	case credentialForegroundOther, credentialForegroundLockScreen:
		return CredentialUIProbeOtherForeground
	default:
		return CredentialUIProbeNoForeground
	}
}

func protectedWindowStillForeground(window uintptr) bool {
	current, _, _ := procGetProtectedForeground.Call()
	return current != 0 && current == window
}

func isCredentialBrokerImage(path string) bool {
	index := strings.LastIndexAny(path, `\/`)
	return strings.EqualFold(path[index+1:], "CredentialUIBroker.exe")
}

func isLockScreenImage(path string) bool {
	index := strings.LastIndexAny(path, `\/`)
	return strings.EqualFold(path[index+1:], "LockApp.exe")
}
