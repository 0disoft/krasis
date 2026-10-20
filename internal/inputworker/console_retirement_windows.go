//go:build windows

package inputworker

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type consoleCloseOps struct {
	wait            func(windows.Handle, uint32) (uint32, error)
	closeHandle     func(windows.Handle) error
	terminateJob    func(windows.Handle, uint32) error
	activeProcesses func(windows.Handle) (uint32, error)
}

var nativeConsoleCloseOps = consoleCloseOps{wait: windows.WaitForSingleObject, closeHandle: windows.CloseHandle, terminateJob: windows.TerminateJobObject, activeProcesses: activeJobProcesses}

func activeJobProcesses(job windows.Handle) (uint32, error) {
	// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION; x/sys exports its class only.
	var accounting struct {
		Times                                                             [4]int64
		TotalPageFaults, TotalProcesses, ActiveProcesses, TotalTerminated uint32
	}
	err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
	return accounting.ActiveProcesses, err
}

func (w *ConsoleWorker) retirementOps() *consoleCloseOps {
	if w.closeOps != nil {
		return w.closeOps
	}
	return &nativeConsoleCloseOps
}

// Only WAIT_TIMEOUT is retryable. A failed wait/close does not establish that
// the numeric handle remains ours; never query/close a potentially reused value.
// Retain that failed owner until service exit, fencing all replacement launches.
func (w *ConsoleWorker) tryRetire(waitMilliseconds uint32) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	if w.poisoned {
		return w.err
	}
	w.retiring, w.err = true, ErrAuthority
	ops := w.retirementOps()
	if w.job != 0 {
		if w.pairJob {
			// Keep the exact job reference until BOTH worker and native child
			// are gone. Closing kill-on-close then waiting only for the parent
			// would allow replacement while its child is still terminating.
			if !w.jobTerminated {
				if ops.terminateJob(w.job, 1) != nil {
					w.poisoned = true
					return w.err
				}
				w.jobTerminated = true
			}
			deadline := time.Now().Add(time.Duration(waitMilliseconds) * time.Millisecond)
			for {
				active, err := ops.activeProcesses(w.job)
				if err != nil {
					w.poisoned = true
					return w.err
				}
				if active == 0 {
					break
				}
				if !time.Now().Before(deadline) {
					return w.err
				}
				time.Sleep(min(10*time.Millisecond, time.Until(deadline)))
			}
		}
		if ops.closeHandle(w.job) != nil {
			w.poisoned = true
			return w.err
		}
		w.job = 0
	}
	if w.process != 0 {
		state, err := ops.wait(w.process, waitMilliseconds)
		if err == nil && state == uint32(windows.WAIT_TIMEOUT) {
			return w.err
		}
		if err != nil || state != windows.WAIT_OBJECT_0 {
			w.poisoned = true
			return w.err
		}
		if ops.closeHandle(w.process) != nil {
			w.poisoned = true
			return w.err
		}
		w.process = 0
	}
	if w.image != nil {
		if w.image.Close() != nil {
			w.poisoned = true
			return w.err
		}
		w.image = nil
	}
	w.closed, w.err = true, nil
	if w.done != nil {
		close(w.done)
	}
	return nil
}

// One owner keeps exact handles and image pins alive after caller failure.
// Reaping only observes exit and closes resources; it cannot clear a missing
// recovery receipt or pressed-state journal. Poisoned owners are never polled.
var consoleRetirements struct {
	sync.Mutex
	pending map[*ConsoleWorker]struct{}
	running bool
}

func workerRetirementPending() bool {
	consoleRetirements.Lock()
	defer consoleRetirements.Unlock()
	return len(consoleRetirements.pending) != 0
}

func retainWorkerRetirement(w *ConsoleWorker) {
	consoleRetirements.Lock()
	defer consoleRetirements.Unlock()
	if consoleRetirements.pending == nil {
		consoleRetirements.pending = make(map[*ConsoleWorker]struct{})
	}
	consoleRetirements.pending[w] = struct{}{}
	if !consoleRetirements.running {
		consoleRetirements.running = true
		go reapConsoleRetirements()
	}
}

func reapConsoleRetirements() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		consoleRetirements.Lock()
		retry := make([]*ConsoleWorker, 0, len(consoleRetirements.pending))
		for w := range consoleRetirements.pending {
			w.mu.Lock()
			poisoned := w.poisoned
			w.mu.Unlock()
			if !poisoned {
				retry = append(retry, w)
			}
		}
		if len(retry) == 0 {
			consoleRetirements.running = false
			consoleRetirements.Unlock()
			return
		}
		consoleRetirements.Unlock()
		for _, w := range retry {
			if w.tryRetire(0) == nil {
				consoleRetirements.Lock()
				delete(consoleRetirements.pending, w)
				consoleRetirements.Unlock()
			}
		}
	}
}
