//go:build windows

package launcher

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func configureProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
