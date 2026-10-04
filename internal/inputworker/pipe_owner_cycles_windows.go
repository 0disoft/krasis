//go:build windows

package inputworker

import (
	"runtime"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
)

var queryThreadCycles = windows.NewLazySystemDLL("kernel32.dll").NewProc("QueryThreadCycleTime")

func currentThreadCycles() (uint64, bool) {
	if queryThreadCycles.Find() != nil {
		return 0, false
	}
	var cycles uint64
	ok, _, _ := queryThreadCycles.Call(uintptr(windows.CurrentThread()), uintptr(unsafe.Pointer(&cycles)))
	return cycles, ok != 0
}

// Optional probe only. Pin the goroutine across both CPU reads and the one
// unchanged kernel owner query; migration would otherwise mix thread totals.
// Wall covers the owner query, cycles also include counter/clock overhead. No
// cycles-to-time conversion, cached PID, extra owner query or error suppression.
func measurePipeOwner(pipe windows.Handle, probe bool) (pid uint32, elapsed time.Duration, cycles uint64, measured bool, err error) {
	var before uint64
	if probe {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		before, measured = currentThreadCycles()
	}
	started := time.Now()
	err = windows.GetNamedPipeServerProcessId(pipe, &pid)
	elapsed = time.Since(started)
	if measured {
		after, ok := currentThreadCycles()
		measured = ok && after >= before && after-before <= handoff.MaxPipeOwnerCycles
		if measured {
			cycles = after - before
		}
	}
	return
}
