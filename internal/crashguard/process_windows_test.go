//go:build windows

package crashguard

import (
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/winmessage"
	"golang.org/x/sys/windows"
)

func TestOwnedGuardWaitDoesNotReportExitOnTimeout(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestCrashGuardHungHelper")
	command.Env = append(os.Environ(), hungHelperEnvironment+"=1")
	configureProcess(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	defer command.Process.Kill()
	var exact uintptr
	if err := command.Process.WithHandle(func(handle uintptr) { exact = handle }); err != nil {
		t.Fatal(err)
	}
	var timeouts atomic.Uint32
	done := make(chan error, 1)
	go func() {
		err := waitOwnedGuard(command.Process, func(handle windows.Handle, ms uint32) (uint32, error) {
			if uintptr(handle) != exact || ms != winmessage.MaximumWaitMillis {
				t.Error("wait changed process identity or native bound")
			}
			state, err := windows.WaitForSingleObject(handle, ms)
			if err == nil && state == uint32(windows.WAIT_TIMEOUT) {
				timeouts.Add(1)
			}
			return state, err
		})
		done <- err
	}()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for timeouts.Load() < 2 {
		select {
		case err := <-done:
			t.Fatalf("live guard reported exit: %v", err)
		case <-deadline.C:
			t.Fatal("native waits did not return independently of child exit")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("exact guard exit was not observed")
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	if command.ProcessState == nil || command.ProcessState.ExitCode() != 0 {
		t.Fatal("exit status was not preserved")
	}
}
