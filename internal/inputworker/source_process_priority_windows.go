//go:build windows

package inputworker

import (
	"sync"

	"golang.org/x/sys/windows"
)

// Only authenticated, single-lease capture/input children enter this scope.
// Their Go scheduler/query-start dependencies otherwise remain NORMAL even when
// native owners have individually promoted threads. Never apply to the service,
// GUI, network agent, presentation or unauthenticated/prepared-only children.
func sourceCaptureProcessSchedulingScope() func() {
	return leaseWorkerProcessSchedulingScope()
}

func leaseWorkerProcessSchedulingScope() func() {
	return sourceProcessPriorityScope(func() uint32 {
		priority, err := windows.GetPriorityClass(windows.CurrentProcess())
		if err != nil {
			return 0
		}
		return priority
	}, func(priority uint32) bool {
		return windows.SetPriorityClass(windows.CurrentProcess(), priority) == nil
	})
}

// Best-effort scheduling only: failures cannot grant authority or request
// service recovery. Preserve explicit priorities and later external changes.
// A failed restoration lasts only until this isolated one-lease child exits.
func sourceProcessPriorityScope(get func() uint32, set func(uint32) bool) func() {
	before := get()
	if before != windows.NORMAL_PRIORITY_CLASS || !set(windows.ABOVE_NORMAL_PRIORITY_CLASS) {
		return func() {}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			if get() == windows.ABOVE_NORMAL_PRIORITY_CLASS {
				set(before)
			}
		})
	}
}
