//go:build windows

package inputworker

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestNativeInputRejectsUninstalledOrIncompleteBootstrap(t *testing.T) {
	if RunInstalledNativeInput(context.Background(), nil) == nil {
		t.Fatal("uninstalled process admitted native input")
	}
	for _, values := range [][]uint64{nil, {0, 1, 2, 3}, {1, 1, 2, 3}, {^uint64(0), 1, 2, 3}} {
		if handles, err := adoptNativeHandles(values); err == nil || handles != nil {
			t.Fatal("invalid inherited bootstrap accepted", values)
		}
	}
	var handles []windows.Handle
	for range 4 {
		handle, err := windows.CreateEvent(nil, 0, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(handle)
		handles = append(handles, handle)
	}
	values := make([]uint64, 4)
	for i, handle := range handles {
		values[i] = uint64(handle)
	}
	if _, err := adoptNativeHandles(values); err == nil {
		t.Fatal("non-inherited handles accepted")
	}
	for _, handle := range handles {
		if windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT) != nil {
			t.Fatal("inheritability setup")
		}
	}
	if _, err := adoptNativeHandles(values); err != nil {
		t.Fatal(err)
	}
	if _, err := adoptNativeHandles(values); err == nil {
		t.Fatal("bootstrap reference adopted twice")
	}
}

func TestConsolePairRetirementRetainsJobUntilEveryProcessExits(t *testing.T) {
	var active atomic.Uint32
	active.Store(1) // The parent has exited; the native child has not.
	var terminated, closed atomic.Int32
	w := &ConsoleWorker{job: 11, process: 22, pairJob: true, done: make(chan struct{}), closeOps: &consoleCloseOps{
		wait:            func(windows.Handle, uint32) (uint32, error) { return windows.WAIT_OBJECT_0, nil },
		closeHandle:     func(windows.Handle) error { closed.Add(1); return nil },
		terminateJob:    func(windows.Handle, uint32) error { terminated.Add(1); return nil },
		activeProcesses: func(windows.Handle) (uint32, error) { return active.Load(), nil },
	}}
	if w.tryRetire(0) == nil || closed.Load() != 0 || terminated.Load() != 1 {
		t.Fatal("parent exit released native-child ownership")
	}
	active.Store(0)
	if w.tryRetire(0) != nil || terminated.Load() != 1 || closed.Load() != 2 {
		t.Fatal("complete pair retirement lost once-only ownership")
	}
	if w.tryRetire(0) != nil || closed.Load() != 2 {
		t.Fatal("completed job was closed twice")
	}
}

func TestNativeConsolePairRetirementKillsAndJoinsBothProcesses(t *testing.T) {
	job, err := makeConsoleWorkerJob(ordinaryConsole)
	if err != nil {
		t.Fatal(err)
	}
	var handles []windows.Handle
	defer func() {
		if job != 0 {
			_ = windows.CloseHandle(job)
		}
		for _, handle := range handles {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
		}
	}()
	for range 2 {
		command := exec.Command(os.Args[0], "-test.run=^TestRetirementNativeChild$")
		command.Env = append(os.Environ(), "KRASIS_RETIREMENT_TEST_CHILD=1")
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		defer command.Wait()
		defer command.Process.Kill()
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
		if err != nil {
			t.Fatal(err)
		}
		handles = append(handles, handle)
		if err := windows.AssignProcessToJobObject(job, handle); err != nil {
			t.Fatal(err)
		}
	}
	w := &ConsoleWorker{job: job, process: handles[0], pairJob: true, done: make(chan struct{})}
	job = 0
	handles[0] = 0
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// w owns the first process and the job; never close their old values again.
	if state, err := windows.WaitForSingleObject(handles[1], 0); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("replacement admitted with native child alive", state, err)
	}
}
