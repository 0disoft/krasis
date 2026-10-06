//go:build windows

package wininput

import (
	"crypto/rand"
	"encoding/hex"
	"runtime"
	"syscall"
	"unsafe"
)

// A window-scoped cookie also detects HWND reuse inside the same process and
// thread. No title, content, queue attachment or window style is inspected.
// UIPI rejection simply disables saved-window restoration for this activation.
type sourceFocusIdentity struct {
	name            []uint16
	process, thread uint32
}

var sourceFocusOwner = func(window uintptr) (uint32, uint32) {
	var process uint32
	thread, _, _ := procGetWindowThreadIDSourceCapture.Call(window, uintptr(unsafe.Pointer(&process)))
	return process, uint32(thread)
}
var sourceFocusProperty = func(window uintptr, name []uint16, operation string) uintptr {
	args := []uintptr{window, uintptr(unsafe.Pointer(&name[0]))}
	if operation == "SetPropW" {
		args = append(args, 1)
	}
	value, _, _ := user32.NewProc(operation).Call(args...)
	runtime.KeepAlive(name)
	return value
}
var sourceFocusRemember = func(window uintptr) sourceFocusIdentity {
	process, thread := sourceFocusOwner(window)
	if process == 0 || thread == 0 {
		return sourceFocusIdentity{}
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return sourceFocusIdentity{}
	}
	name, _ := syscall.UTF16FromString("Krasis.Focus." + hex.EncodeToString(nonce[:]))
	identity := sourceFocusIdentity{name: name, process: process, thread: thread}
	if sourceFocusProperty(window, name, "SetPropW") == 0 {
		return sourceFocusIdentity{}
	}
	if !sourceFocusMatches(window, identity) {
		sourceFocusForget(window, identity)
		return sourceFocusIdentity{}
	}
	return identity
}
var sourceFocusMatches = func(window uintptr, identity sourceFocusIdentity) bool {
	if window == 0 || len(identity.name) == 0 || !sourceWheelCaptureIsWindow(window) {
		return false
	}
	process, thread := sourceFocusOwner(window)
	return process == identity.process && thread == identity.thread &&
		sourceFocusProperty(window, identity.name, "GetPropW") == 1
}
var sourceFocusForget = func(window uintptr, identity sourceFocusIdentity) {
	if window != 0 && len(identity.name) != 0 && sourceFocusProperty(window, identity.name, "GetPropW") == 1 {
		sourceFocusProperty(window, identity.name, "RemovePropW")
	}
}

func (current *sourceWheelCaptureRun) forgetSavedFocus() {
	sourceFocusForget(current.savedFocus, current.savedFocusIdentity)
	current.savedFocusIdentity = sourceFocusIdentity{}
	current.savedFocus = 0
}
