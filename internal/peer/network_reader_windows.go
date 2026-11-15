//go:build windows

package peer

import (
	"runtime"

	"golang.org/x/sys/windows"
)

const networkReadAboveNormal int32 = 1

var networkKernel = windows.NewLazySystemDLL("kernel32.dll")
var networkGetPriority = networkKernel.NewProc("GetThreadPriority")
var networkSetPriority = networkKernel.NewProc("SetThreadPriority")

func networkThreadPriority() int32 {
	if networkGetPriority.Find() != nil {
		return 0x7fffffff
	}
	value, _, _ := networkGetPriority.Call(uintptr(windows.CurrentThread()))
	return int32(value)
}

func setNetworkThreadPriority(value int32) bool {
	if networkSetPriority.Find() != nil {
		return false
	}
	result, _, _ := networkSetPriority.Call(uintptr(windows.CurrentThread()), uintptr(value))
	return result != 0
}

// One authenticated wire reader per connection, never a thread per frame.
// A best-effort scheduling hint only: retain FIFO, heartbeat admission, bounded
// queues, cancellation, deadlines and joining. Do not override explicit policy.
func runNetworkReader(read func()) {
	runtime.LockOSThread()
	restore := networkReaderPriorityScope(networkThreadPriority, setNetworkThreadPriority)
	defer func() {
		if restore() {
			runtime.UnlockOSThread()
		}
		// A failed restoration keeps the OS thread locked through goroutine
		// exit; the runtime retires it instead of reusing a raised thread.
	}()
	read()
}

func networkReaderPriorityScope(get func() int32, set func(int32) bool) func() bool {
	before := get()
	raised := before == 0 && set(networkReadAboveNormal)
	return func() bool { return !raised || set(before) }
}
