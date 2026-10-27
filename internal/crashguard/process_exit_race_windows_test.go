//go:build windows

package crashguard

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestUnboundRetirementAcceptsAlreadyExitedWindowsChild(t *testing.T) {
	// Leave the real, signaled process unreaped until after the abort. This
	// reproduces TerminateProcess(5) without inventing an exit acknowledgement.
	command := exec.Command(os.Args[0], "-test.run=^$")
	configureProcess(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	var state uint32
	var waitErr error
	if err := command.Process.WithHandle(func(handle uintptr) {
		state, waitErr = windows.WaitForSingleObject(windows.Handle(handle), 2000)
	}); err != nil || waitErr != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("test child did not exit", err, waitErr, state)
	}
	if err := command.Process.Kill(); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatal("expected the Windows already-exited termination race", err)
	}
	owner := closeTestProcess(t)
	owner.process = command.Process
	<-owner.requestAbort()
	if !errors.Is(owner.abortErr, windows.ERROR_ACCESS_DENIED) {
		t.Fatal("test did not retain the native termination denial", owner.abortErr)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	owner.exited = command.ProcessState != nil
	close(owner.done)
	if err := owner.closeUnbound(); err != nil {
		t.Fatal("confirmed unbound exit became a recovery failure", err)
	}
}

func TestUnboundRetirementKeepsUnknownExitFailure(t *testing.T) {
	owner := closeTestProcess(t)
	owner.kill = func() error { return windows.ERROR_ACCESS_DENIED }
	close(owner.done) // No ProcessState: this is not proof of native exit.
	if err := owner.closeUnbound(); !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		t.Fatal("unknown child lost its termination failure", err)
	}
	if err := confirmedExitAbort(syscall.EINVAL, false); !errors.Is(err, syscall.EINVAL) {
		t.Fatal("unconfirmed released handle fabricated child exit", err)
	}
	other := errors.New("other termination failure")
	if err := confirmedExitAbort(other, true); err != other {
		t.Fatal("unrelated termination failure was changed", err)
	}
}

func TestWaitRecordsExactWindowsChildExit(t *testing.T) {
	owner, err := startCommand(exec.Command(os.Args[0], "-test.run=^$"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.closeUnbound() })
	select {
	case <-owner.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("child exit was not joined")
	}
	owner.mu.Lock()
	exited := owner.exited
	owner.mu.Unlock()
	if !exited {
		t.Fatal("wait discarded the exact child's ProcessState")
	}
	if err := owner.closeUnbound(); err != nil {
		t.Fatal(err)
	}
}
