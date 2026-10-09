//go:build windows

package launchrecord

import (
	"os/exec"
	"path/filepath"
	"syscall"
)

func Start(path string) error {
	return startWithArgs(path)
}
func StartAtLogin(path string) error         { return startWithArgs(path, "--startup") }
func StartRecovery(path, token string) error { return startWithArgs(path, "--recover", token) }
func StartAfterInstall(path string) error    { return startWithArgs(path, "--installed") }
func startWithArgs(path string, args ...string) error {
	return StartIndependent(launchCommand(path, args...))
}

func launchCommand(path string, args ...string) *exec.Cmd {
	command := exec.Command(path, args...)
	command.Dir = filepath.Dir(path)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000208}
	command.Env = windowsLaunchEnvironment()
	return command
}
