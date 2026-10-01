//go:build windows

package launchrecord

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func Start(path string) error {
	command := exec.Command(path)
	command.Dir = filepath.Dir(path)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000208}
	for _, key := range []string{"SystemRoot", "WINDIR", "LOCALAPPDATA", "TEMP", "TMP", "USERPROFILE"} {
		if value, ok := os.LookupEnv(key); ok {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
