//go:build windows

package inputworker

import (
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

type consoleCloseOps struct {
	wait        func(windows.Handle, uint32) (uint32, error)
	closeHandle func(windows.Handle) error
}

var nativeConsoleCloseOps = consoleCloseOps{wait: windows.WaitForSingleObject, closeHandle: windows.CloseHandle}

func (w *ConsoleWorker) retirementOps() *consoleCloseOps {
	if w.closeOps != nil {
		return w.closeOps
	}
	return &nativeConsoleCloseOps
}

// An unsuccessful join retains both the exact process handle and image pin.
// A timeout is never process exit, and cannot authorize another native owner.
func (w *ConsoleWorker) tryRetire(waitMilliseconds uint32) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.err = ErrAuthority
	ops := w.retirementOps()
	if w.job != 0 {
		if ops.closeHandle(w.job) != nil {
			return w.err
		}
		w.job = 0
	}
	if w.process != 0 {
		state, err := ops.wait(w.process, waitMilliseconds)
		if err != nil || state != windows.WAIT_OBJECT_0 {
			return w.err
		}
		if ops.closeHandle(w.process) != nil {
			return w.err
		}
		w.process = 0
	}
	if w.image != nil {
		if w.image.Close() != nil {
			return w.err
		}
		w.image = nil
	}
	w.closed = true
	w.err = nil
	if w.done != nil {
		close(w.done)
	}
	return nil
}

// A single process-wide owner retains failed retirements. Launch is fenced
// while this set is nonempty. Reaping only joins/closes; it never injects,
// replays input, fabricates recovery receipts or clears service recovery state.
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
		workers := make([]*ConsoleWorker, 0, len(consoleRetirements.pending))
		for w := range consoleRetirements.pending {
			workers = append(workers, w)
		}
		consoleRetirements.Unlock()
		for _, w := range workers {
			if w.tryRetire(0) == nil {
				consoleRetirements.Lock()
				delete(consoleRetirements.pending, w)
				consoleRetirements.Unlock()
			}
		}
		consoleRetirements.Lock()
		if len(consoleRetirements.pending) == 0 {
			consoleRetirements.running = false
			consoleRetirements.Unlock()
			return
		}
		consoleRetirements.Unlock()
	}
}
