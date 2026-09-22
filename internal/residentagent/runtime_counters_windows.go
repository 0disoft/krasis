//go:build windows

package residentagent

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

var getRuntimeMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

type processMemoryCounters struct {
	Size, PageFaultCount                                                                 uint32
	PeakWorkingSetSize, WorkingSetSize, QuotaPeakPagedPoolUsage, QuotaPagedPoolUsage     uintptr
	QuotaPeakNonPagedPoolUsage, QuotaNonPagedPoolUsage, PagefileUsage, PeakPagefileUsage uintptr
}

func processRuntimeCounters() (workingSet uint64, memoryKnown bool, cpuMicros uint64, cpuKnown bool) {
	process := windows.CurrentProcess()
	var memory processMemoryCounters
	memory.Size = uint32(unsafe.Sizeof(memory))
	if getRuntimeMemoryInfo.Find() == nil {
		ok, _, _ := getRuntimeMemoryInfo.Call(uintptr(process), uintptr(unsafe.Pointer(&memory)), uintptr(memory.Size))
		if ok != 0 {
			workingSet = uint64(memory.WorkingSetSize)
			memoryKnown = true
		}
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(process, &created, &exited, &kernel, &user) == nil {
		ticks := func(v windows.Filetime) uint64 { return uint64(v.HighDateTime)<<32 | uint64(v.LowDateTime) }
		cpuMicros = (ticks(kernel) + ticks(user)) / 10
		cpuKnown = true
	}
	return
}
