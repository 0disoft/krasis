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
	if !inputDesktopDefault() {
		return true
	}
	return credentialBrokerForeground(true)
}

// CredentialBrokerForeground is deliberately narrower than ProtectedInputActive:
// the probe must never send input to a non-default or unidentifiable desktop.
func CredentialBrokerForeground() bool {
	return inputDesktopDefault() && credentialBrokerForeground(false)
}

func inputDesktopDefault() bool {
	desktop, _, _ := procOpenInputDesktop.Call(0, 0, 0x0001) // DESKTOP_READOBJECTS
	if desktop == 0 {
		return false
	}
	var name [128]uint16
	var needed uint32
	ok, _, _ := procGetUserObjectInformation.Call(desktop, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed))) // UOI_NAME
	procCloseDesktop.Call(desktop)
	return ok != 0 && syscall.UTF16ToString(name[:]) == "Default"
}

// uncertainAsProtected keeps the normal session guard conservative, while the
// attended probe requires a positively identified broker before injecting.
func credentialBrokerForeground(uncertainAsProtected bool) bool {
	state := inspectCredentialForeground()
	return state == credentialForegroundBroker || (uncertainAsProtected && state == credentialForegroundUnknown)
}

type credentialForegroundState uint8

const (
	credentialForegroundMissing credentialForegroundState = iota
	credentialForegroundUnknown
	credentialForegroundOther
	credentialForegroundBroker
)

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
	case credentialForegroundOther:
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
