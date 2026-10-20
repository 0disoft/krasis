//go:build windows

package inputworker

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func waitConsoleRetirement(t *testing.T, w *ConsoleWorker) {
	t.Helper()
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
		t.Fatal("retained worker did not retire")
	}
	deadline := time.Now().Add(time.Second)
	for workerRetirementPending() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if workerRetirementPending() {
		t.Fatal("retired worker still fenced launches")
	}
}

// Propagation-only tests need a completed failed Close attempt, not a forever
// invalid native handle retained in the process-wide retirement owner.
func failedConsoleCloseForTest() *ConsoleWorker {
	w := &ConsoleWorker{err: ErrAuthority, done: make(chan struct{})}
	w.once.Do(func() {})
	return w
}

func TestFailedConsoleRetirementRetainsOwnershipAndFencesLaunch(t *testing.T) {
	for _, stage := range []string{"wait_timeout", "wait_error", "job_close", "process_close"} {
		t.Run(stage, func(t *testing.T) {
			pin, err := os.CreateTemp(t.TempDir(), "image-pin")
			if err != nil {
				t.Fatal(err)
			}
			var allow atomic.Bool
			var jobs, processes atomic.Uint32
			w := &ConsoleWorker{job: 11, process: 22, image: pin, done: make(chan struct{})}
			w.closeOps = &consoleCloseOps{
				wait: func(windows.Handle, uint32) (uint32, error) {
					if !allow.Load() {
						if stage == "wait_timeout" {
							return uint32(windows.WAIT_TIMEOUT), nil
						}
						if stage == "wait_error" {
							return 0, windows.ERROR_INVALID_HANDLE
						}
					}
					return windows.WAIT_OBJECT_0, nil
				},
				closeHandle: func(h windows.Handle) error {
					if !allow.Load() && (h == 11 && stage == "job_close" || h == 22 && stage == "process_close") {
						return windows.ERROR_ACCESS_DENIED
					}
					if h == 11 {
						jobs.Add(1)
					} else if h == 22 {
						processes.Add(1)
					} else {
						t.Error("unknown handle closed")
					}
					return nil
				},
			}
			defer func() { allow.Store(true); waitConsoleRetirement(t, w) }()
			if w.closeWithin(0) == nil || !workerRetirementPending() {
				t.Fatal("unconfirmed retirement accepted")
			}
			w.mu.Lock()
			retained := !w.closed && w.process == 22 && w.image == pin
			w.mu.Unlock()
			if !retained {
				t.Fatal("native ownership lost before retirement")
			}
			if _, err := pin.Stat(); err != nil {
				t.Fatal("image pin released before confirmed retirement", err)
			}
			select {
			case <-w.done:
				t.Fatal("unknown retirement signalled done")
			default:
			}
			if _, err := launchManagedConsoleMode(context.Background(), [32]byte{}, nil, nil, ordinaryConsole, func() bool { t.Fatal("pending retirement reached launch authorization"); return true }); !errors.Is(err, ErrRecovery) {
				t.Fatal("replacement launch was not fenced", err)
			}
			allow.Store(true)
			waitConsoleRetirement(t, w)
			if err := w.Close(); err != nil || jobs.Load() != 1 || processes.Load() != 1 {
				t.Fatal("late retirement leaked or double-closed ownership", err, jobs.Load(), processes.Load())
			}
			if _, err := pin.Stat(); err == nil {
				t.Fatal("confirmed retirement retained image pin")
			}
		})
	}
}

func TestConsoleRetirementChild(t *testing.T) {
	if os.Getenv("KRASIS_RETIREMENT_TEST_CHILD") != "1" {
		return
	}
	time.Sleep(time.Minute)
}

func TestNativeConsoleRetirementKeepsExactProcessUntilObservedExit(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestConsoleRetirementChild$")
	command.Env = append(os.Environ(), "KRASIS_RETIREMENT_TEST_CHILD=1")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	job, err := makeWorkerJob()
	if err != nil {
		windows.CloseHandle(process)
		t.Fatal(err)
	}
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		windows.CloseHandle(process)
		t.Fatal(err)
	}
	pin, err := os.Open(os.Args[0])
	if err != nil {
		windows.CloseHandle(job)
		windows.CloseHandle(process)
		t.Fatal(err)
	}
	var observe atomic.Bool
	w := &ConsoleWorker{job: job, process: process, image: pin, done: make(chan struct{}), closeOps: &consoleCloseOps{
		wait: func(h windows.Handle, ms uint32) (uint32, error) {
			if !observe.Load() {
				return uint32(windows.WAIT_TIMEOUT), nil
			}
			return windows.WaitForSingleObject(h, ms)
		}, closeHandle: windows.CloseHandle,
	}}
	defer func() { observe.Store(true); waitConsoleRetirement(t, w) }()
	if w.closeWithin(0) == nil || w.exited() {
		t.Fatal("unobserved native exit accepted")
	}
	if _, err := pin.Stat(); err != nil {
		t.Fatal("real image pin released early")
	}
	if _, err := windows.WaitForSingleObject(process, 0); err != nil {
		t.Fatal("exact kernel process handle was lost", err)
	}
	observe.Store(true)
	waitConsoleRetirement(t, w)
	if w.Close() != nil || !w.exited() {
		t.Fatal("late native join did not finish retirement")
	}
}
