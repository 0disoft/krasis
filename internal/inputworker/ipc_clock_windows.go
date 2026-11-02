//go:build windows

package inputworker

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var ipcCounter = windows.NewLazySystemDLL("kernel32.dll").NewProc("QueryPerformanceCounter")
var ipcFrequency = windows.NewLazySystemDLL("kernel32.dll").NewProc("QueryPerformanceFrequency")
var ipcClockFrequency = sync.OnceValue(func() int64 {
	var frequency int64
	ok, _, _ := ipcFrequency.Call(uintptr(unsafe.Pointer(&frequency)))
	if ok == 0 || frequency <= 0 || frequency > 1_000_000_000 {
		return 0
	}
	return frequency
})

// QPC has a common epoch/frequency across this Windows PC's processes. A clock
// failure drops optional diagnostics; it never rejects or authorizes input.
func workerIPCTick() uint64 {
	frequency := ipcClockFrequency()
	if frequency == 0 {
		return 0
	}
	var counter int64
	ok, _, _ := ipcCounter.Call(uintptr(unsafe.Pointer(&counter)))
	if ok == 0 || counter <= 0 {
		return 0
	}
	return uint64(counter/frequency)*1_000_000 + uint64(counter%frequency)*1_000_000/uint64(frequency)
}
