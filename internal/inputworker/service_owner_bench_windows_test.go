//go:build windows

package inputworker

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var benchGetThreadTimes = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetThreadTimes")

func benchThreadCPU(b *testing.B) uint64 {
	b.Helper()
	var creation, exit, kernel, user windows.Filetime
	if benchGetThreadTimes.Find() != nil {
		b.Fatal("thread CPU query unavailable")
	}
	ok, _, _ := benchGetThreadTimes.Call(uintptr(windows.CurrentThread()), uintptr(unsafe.Pointer(&creation)), uintptr(unsafe.Pointer(&exit)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if ok == 0 {
		b.Fatal("thread CPU query failed")
	}
	return (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) + (uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
}

// Temporary pipe only, no product service, token change or input. Pin the
// benchmark goroutine once so aggregate kernel+user CPU refers to one thread.
// Per-call wall maxima include preemption/blocking; CPU is aggregate, not a
// per-call or per-frame attribution and has Windows accounting granularity.
func BenchmarkNativePipeOwner(b *testing.B) {
	_, client := nativePair(b)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	expected := windows.GetCurrentProcessId()
	var warm uint32
	if windows.GetNamedPipeServerProcessId(client.handle, &warm) != nil || warm != expected {
		b.Fatal("native fixture owner unavailable")
	}
	cpuStart := benchThreadCPU(b)
	var maximum time.Duration
	for b.Loop() {
		started := time.Now()
		var pid uint32
		err := windows.GetNamedPipeServerProcessId(client.handle, &pid)
		elapsed := time.Since(started)
		if err != nil || pid != expected {
			b.Fatal("native fixture owner changed")
		}
		if elapsed > maximum {
			maximum = elapsed
		}
	}
	cpuEnd := benchThreadCPU(b)
	if cpuEnd < cpuStart {
		b.Fatal("thread CPU accounting moved backwards")
	}
	b.ReportMetric(float64(cpuEnd-cpuStart)*100/float64(b.N), "thread-cpu-ns/op")
	b.ReportMetric(float64(maximum.Nanoseconds()), "max-wall-ns")
}

// Candidate diagnostic overhead: both counters and the per-query thread pin
// are included in ns/op and aggregate thread CPU. No cycles-to-time conversion.
func BenchmarkNativePipeOwnerCycleProbe(b *testing.B) {
	_, client := nativePair(b)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	expected := windows.GetCurrentProcessId()
	if _, _, _, measured, err := measurePipeOwner(client.handle, true); err != nil || !measured {
		b.Fatal("thread cycle query unavailable")
	}
	cpuStart := benchThreadCPU(b)
	var maximum time.Duration
	var totalCycles uint64
	for b.Loop() {
		pid, elapsed, cycles, measured, err := measurePipeOwner(client.handle, true)
		if err != nil || pid != expected || !measured {
			b.Fatal("native probe failed")
		}
		totalCycles += cycles
		if elapsed > maximum {
			maximum = elapsed
		}
	}
	cpuEnd := benchThreadCPU(b)
	if cpuEnd < cpuStart {
		b.Fatal("thread CPU accounting moved backwards")
	}
	b.ReportMetric(float64(cpuEnd-cpuStart)*100/float64(b.N), "thread-cpu-ns/op")
	b.ReportMetric(float64(totalCycles)/float64(b.N), "cycles/op")
	b.ReportMetric(float64(maximum.Nanoseconds()), "max-wall-ns")
}
