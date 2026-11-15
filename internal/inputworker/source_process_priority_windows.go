//go:build windows

package inputworker

import (
	"sync"

	"golang.org/x/sys/windows"
)

// Only the authenticated, single-lease capture child may enter this scope.
// Its Go scheduler/query-start dependencies otherwise remain NORMAL even when
// the native capture/IPC owner threads have been individually promoted.
// Never apply this to the service, GUI, network agent or other worker modes.
func sourceCaptureProcessSchedulingScope() func() {
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
