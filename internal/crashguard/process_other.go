//go:build !windows

package crashguard

import "os/exec"

func confirmedExitAbort(err error, _ bool) error { return err }

func configureProcess(_ *exec.Cmd) {}

func waitProcess(command *exec.Cmd) error { return command.Wait() }

func configureStderr(command *exec.Cmd, sink *limitedBuffer) (func(), func() error, error) {
	command.Stderr = sink
	return func() {}, func() error { return nil }, nil
}
