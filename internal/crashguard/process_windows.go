//go:build windows

package crashguard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/0disoft/krasis/internal/winmessage"
	"golang.org/x/sys/windows"
)

func configureProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.HideWindow = true
}

// Windows also returns ACCESS_DENIED when an exact process has already exited,
// before our waiter reaps it. Go reports EINVAL if Wait already released its
// handle. Only that child's ProcessState proves its exit; a released handle
// alone cannot. Call after joining exit and pipe cleanup.
func confirmedExitAbort(terminationErr error, exited bool) error {
	if exited && (errors.Is(terminationErr, windows.ERROR_ACCESS_DENIED) || errors.Is(terminationErr, syscall.EINVAL)) {
		return nil
	}
	return terminationErr
}

func waitProcess(command *exec.Cmd) error {
	// os.Process.Wait uses an infinite Windows wait. First wait in bounded
	// native calls so Go's GC can take a missed syscall P (go#81759). Once
	// signalled, the process handle stays signalled and Cmd.Wait can reap it
	// immediately while retaining its normal exit status and pipe cleanup.
	err := waitOwnedGuard(command.Process, windows.WaitForSingleObject)
	// On an unexpected native failure, retain the real error and still reap
	// the owned child. Never close Done while its exit remains unconfirmed.
	return errors.Join(err, command.Wait())
}

func waitOwnedGuard(process *os.Process, wait func(windows.Handle, uint32) (uint32, error)) error {
	var waitErr error
	err := process.WithHandle(func(handle uintptr) {
		// WithHandle pins the exact process; no PID lookup or duplicate handle
		// is needed, and concurrent Kill does not invalidate this reference.
		for {
			state, err := wait(windows.Handle(handle), winmessage.MaximumWaitMillis)
			if err != nil {
				waitErr = fmt.Errorf("wait for guard exit: %w", err)
				return
			}
			switch state {
			case windows.WAIT_OBJECT_0:
				return
			case uint32(windows.WAIT_TIMEOUT):
				continue
			default:
				waitErr = fmt.Errorf("unexpected guard wait result %d", state)
				return
			}
		}
	})
	return errors.Join(err, waitErr)
}
