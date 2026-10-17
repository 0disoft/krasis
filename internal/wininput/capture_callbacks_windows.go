//go:build windows

package wininput

import (
	"errors"
	"unsafe"
)

// Capture uses the same process-lifetime trampolines as the ordinary source
// hook. Only these collectable handlers retain session counters; none is passed
// to NewCallback. The dedicated locked OS thread owns registration and removal.
func registerCaptureHookOwner(thread uintptr, counters *captureCounters) (func(), error) {
	if thread == 0 || counters == nil {
		return nil, errors.New("capture hook thread and counters are required")
	}
	owner := &sourceHookCallbacks{
		keyboard: func(code int, w, l uintptr) uintptr {
			if code == hcAction && l != 0 {
				data := (*keyboardHookData)(unsafe.Pointer(l))
				counters.observeKeyboard(uint32(w), data.Flags&llkhfInjected != 0)
			}
			return callNextHook(code, w, l)
		},
		mouse: func(code int, w, l uintptr) uintptr {
			if code == hcAction && l != 0 {
				data := (*mouseHookData)(unsafe.Pointer(l))
				counters.observeMouse(uint32(w), data.Flags&llmhfInjected != 0, data.ExtraInfo)
			}
			return callNextHook(code, w, l)
		},
	}
	if _, loaded := sourceHookOwners.LoadOrStore(thread, owner); loaded {
		return nil, errors.New("capture hook thread already has an owner")
	}
	return func() { sourceHookOwners.CompareAndDelete(thread, owner) }, nil
}
