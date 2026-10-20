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

// These tests propagate a previously confirmed cleanup error; they do not own
// native resources and must not register an invalid handle in the live reaper.
func failedConsoleCloseForTest() *ConsoleWorker {
	w := &ConsoleWorker{err: ErrAuthority, done: make(chan struct{})}
	w.once.Do(func() {})
	return w
}

func awaitConsoleRetirement(t *testing.T, w *ConsoleWorker) {
	t.Helper()
	select {
	case <-w.done:
	case <-time.After(3 * time.Second):
		t.Fatal("retained worker did not retire")
	}
	limit := time.Now().Add(time.Second)
	for workerRetirementPending() && time.Now().Before(limit) {
		time.Sleep(time.Millisecond)
	}
	if workerRetirementPending() {
		t.Fatal("completed retirement still fenced launches")
	}
}

func TestConsoleWaitTimeoutRetainsOwnershipUntilExactExit(t *testing.T) {
	pin, err := os.CreateTemp(t.TempDir(), "image-pin")
	if err != nil {
		t.Fatal(err)
	}
	var exited atomic.Bool
	var jobs, processes atomic.Int32
	w := &ConsoleWorker{job: 11, process: 22, image: pin, done: make(chan struct{}), closeOps: &consoleCloseOps{
		wait: func(windows.Handle, uint32) (uint32, error) {
			if !exited.Load() {
				return uint32(windows.WAIT_TIMEOUT), nil
			}
			return windows.WAIT_OBJECT_0, nil
		},
		closeHandle: func(h windows.Handle) error {
			if h == 11 {
				jobs.Add(1)
			} else if h == 22 {
				processes.Add(1)
			} else {
				t.Error("unowned handle closed")
			}
			return nil
		},
	}}
	defer func() { exited.Store(true); awaitConsoleRetirement(t, w) }()
	if w.closeWithin(0) == nil || w.exited() || !workerRetirementPending() {
		t.Fatal("timeout was treated as native retirement")
	}
	if _, err := pin.Stat(); err != nil || processes.Load() != 0 {
		t.Fatal("unconfirmed child lost its image/process pin", err)
	}
	if _, err := launchManagedConsoleMode(context.Background(), [32]byte{}, nil, nil, ordinaryConsole, func() bool { t.Fatal("retirement fence admitted launch"); return true }); !errors.Is(err, ErrRecovery) {
		t.Fatal("replacement owner admitted during unconfirmed retirement", err)
	}
	exited.Store(true)
	awaitConsoleRetirement(t, w)
	if w.Close() != nil || jobs.Load() != 1 || processes.Load() != 1 {
		t.Fatal("late retirement double-closed ownership")
	}
}

func TestConsoleHandleFailuresAreNeverRetried(t *testing.T) {
	for _, stage := range []string{"job_close", "wait", "unexpected_wait", "process_close"} {
		t.Run(stage, func(t *testing.T) {
			pin, err := os.CreateTemp(t.TempDir(), "image-pin")
			if err != nil {
				t.Fatal(err)
			}
			var waits, closes atomic.Int32
			w := &ConsoleWorker{job: 11, process: 22, image: pin, done: make(chan struct{}), closeOps: &consoleCloseOps{
				wait: func(windows.Handle, uint32) (uint32, error) {
					waits.Add(1)
					if stage == "wait" {
						return 0, windows.ERROR_INVALID_HANDLE
					}
					if stage == "unexpected_wait" {
						return windows.WAIT_ABANDONED, nil
					}
					return windows.WAIT_OBJECT_0, nil
				},
				closeHandle: func(h windows.Handle) error {
					closes.Add(1)
					if stage == "job_close" && h == 11 || stage == "process_close" && h == 22 {
						return windows.ERROR_INVALID_HANDLE
					}
					return nil
				},
			}}
			// Synthetic owners have no kernel resources. Remove only this test's
			// poisoned record; production retains poisoned ownership until exit.
			defer func() {
				consoleRetirements.Lock()
				delete(consoleRetirements.pending, w)
				consoleRetirements.Unlock()
				_ = pin.Close()
			}()
			if w.closeWithin(0) == nil || !workerRetirementPending() {
				t.Fatal("failed native retirement was accepted")
			}
			beforeWaits, beforeCloses := waits.Load(), closes.Load()
			for range 10 {
				if w.tryRetire(0) == nil || w.Close() == nil || w.exited() {
					t.Fatal("failed ownership recovered without evidence")
				}
			}
			time.Sleep(300 * time.Millisecond) // Include the automatic reaper.
			if waits.Load() != beforeWaits || closes.Load() != beforeCloses {
				t.Fatal("failed handle value queried or closed again")
			}
			if _, err := pin.Stat(); err != nil {
				t.Fatal("poisoned retirement dropped its image pin", err)
			}
			select {
			case <-w.done:
				t.Fatal("failed retirement signaled completion")
			default:
			}
		})
	}
}

func TestRetirementNativeChild(t *testing.T) {
	if os.Getenv("KRASIS_RETIREMENT_TEST_CHILD") == "1" {
		time.Sleep(time.Minute)
	}
}

func TestNativeRetirementKeepsProcessAndImageUntilObservedExit(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestRetirementNativeChild$")
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
		_ = windows.CloseHandle(process)
		t.Fatal(err)
	}
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		_ = windows.CloseHandle(job)
		_ = windows.CloseHandle(process)
		t.Fatal(err)
	}
	pin, err := os.Open(os.Args[0])
	if err != nil {
		_ = windows.CloseHandle(job)
		_ = windows.CloseHandle(process)
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
	defer func() { observe.Store(true); awaitConsoleRetirement(t, w) }()
	if w.closeWithin(0) == nil || w.exited() {
		t.Fatal("unobserved process exit accepted")
	}
	if _, err := pin.Stat(); err != nil {
		t.Fatal("image pin released before exit observation")
	}
	if _, err := windows.WaitForSingleObject(process, 0); err != nil {
		t.Fatal("exact process handle was lost", err)
	}
	observe.Store(true)
	awaitConsoleRetirement(t, w)
	if w.Close() != nil || !w.exited() {
		t.Fatal("observed retirement did not finish")
	}
}
