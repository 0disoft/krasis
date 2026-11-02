//go:build windows

package residentagent

import (
	"golang.org/x/sys/windows"
	"time"
	"unsafe"
)

var getRuntimeMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
var getRuntimeSystemTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetSystemTimes")

type processMemoryCounters struct {
	Size, PageFaultCount                                                                 uint32
	PeakWorkingSetSize, WorkingSetSize, QuotaPeakPagedPoolUsage, QuotaPagedPoolUsage     uintptr
	QuotaPeakNonPagedPoolUsage, QuotaNonPagedPoolUsage, PagefileUsage, PeakPagefileUsage uintptr
}

func processRuntimeCounters() (workingSet uint64, memoryKnown bool, cpuMicros uint64, cpuKnown bool) {
	memory, memoryKnown, cpuMicros, cpuKnown := readRuntimeProcessCounters()
	return uint64(memory.WorkingSetSize), memoryKnown, cpuMicros, cpuKnown
}

func readRuntimeProcessCounters() (memory processMemoryCounters, memoryKnown bool, cpuMicros uint64, cpuKnown bool) {
	process := windows.CurrentProcess()
	memory.Size = uint32(unsafe.Sizeof(memory))
	if getRuntimeMemoryInfo.Find() == nil {
		ok, _, _ := getRuntimeMemoryInfo.Call(uintptr(process), uintptr(unsafe.Pointer(&memory)), uintptr(memory.Size))
		if ok != 0 {
			memoryKnown = true
		}
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(process, &created, &exited, &kernel, &user) == nil {
		cpuMicros = (runtimeFiletimeTicks(kernel) + runtimeFiletimeTicks(user)) / 10
		cpuKnown = true
	}
	return
}

func runtimeFiletimeTicks(v windows.Filetime) uint64 {
	return uint64(v.HighDateTime)<<32 | uint64(v.LowDateTime)
}

func processGCOSCounters() gcOSCounters {
	started := time.Now()
	memory, memoryKnown, cpu, cpuKnown := readRuntimeProcessCounters()
	s := gcOSCounters{cpu: cpu, cpuKnown: cpuKnown, faults: uint64(memory.PageFaultCount), faultsKnown: memoryKnown}
	var idle, kernel, user windows.Filetime
	if getRuntimeSystemTimes.Find() == nil {
		ok, _, _ := getRuntimeSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
		if ok != 0 {
			s.systemIdle, s.systemKernel, s.systemUser = runtimeFiletimeTicks(idle), runtimeFiletimeTicks(kernel), runtimeFiletimeTicks(user)
			s.systemKnown = true
		}
	}
	s.readMicros = uint64(max(0, time.Since(started).Microseconds()))
	return s
}
