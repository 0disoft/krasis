//go:build linux

package launchrecord

import (
	"os/exec"
	"path/filepath"
	"syscall"
)

// StartIndependent detaches from the launching terminal's process session.
// Reap in the background instead of releasing the process and leaving a zombie
// when a long-lived CLI or controller remains alive after its child exits.
func StartIndependent(command *exec.Cmd) error {
	if command == nil || !filepath.IsAbs(command.Path) || !filepath.IsAbs(command.Dir) || len(command.Args) == 0 {
		return ErrInvalid
	}
	attributes := syscall.SysProcAttr{}
	if command.SysProcAttr != nil {
		attributes = *command.SysProcAttr
	}
	attributes.Setsid = true
	attributes.Setpgid = false
	command.SysProcAttr = &attributes
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
