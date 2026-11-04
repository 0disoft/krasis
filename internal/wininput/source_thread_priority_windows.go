//go:build windows

package wininput

import "golang.org/x/sys/windows"

const (
	sourceInputPriorityNormal      int32 = 0
	sourceInputPriorityAboveNormal int32 = 1
	sourceInputPriorityUnknown     int32 = 0x7fffffff
)

var sourceGetThreadPriority = kernel32.NewProc("GetThreadPriority")
var sourceSetThreadPriority = kernel32.NewProc("SetThreadPriority")

func currentSourceThreadPriority() int32 {
	if sourceGetThreadPriority.Find() != nil {
		return sourceInputPriorityUnknown
	}
	value, _, _ := sourceGetThreadPriority.Call(uintptr(windows.CurrentThread()))
	return int32(value)
}

func setSourceThreadPriority(priority int32) bool {
	if sourceSetThreadPriority.Find() != nil {
		return false
	}
	value, _, _ := sourceSetThreadPriority.Call(uintptr(windows.CurrentThread()), uintptr(priority))
	return value != 0
}

// A best-effort scheduling hint, never authority/readiness/retirement proof.
// Preserve an explicitly lower/higher priority and unknown/native failures.
func promoteSourceInputThread(get func() int32, set func(int32) bool) bool {
	if get() != sourceInputPriorityNormal {
		return false
	}
	return set(sourceInputPriorityAboveNormal)
}
