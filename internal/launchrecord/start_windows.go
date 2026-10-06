//go:build windows

package launchrecord

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func Start(path string) error {
	return startWithArgs(path)
}
func StartAtLogin(path string) error { return startWithArgs(path, "--startup") }
func startWithArgs(path string, args ...string) error {
	command := exec.Command(path, args...)
	command.Dir = filepath.Dir(path)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000208}
	for _, key := range []string{"SystemRoot", "WINDIR", "LOCALAPPDATA", "TEMP", "TMP", "USERPROFILE"} {
		if value, ok := os.LookupEnv(key); ok {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	return StartIndependent(command)
}
