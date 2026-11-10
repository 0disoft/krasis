//go:build linux

package launchrecord

import (
	"errors"
	"os/exec"
	"path/filepath"
)

var ErrUnsupported = errors.New("Windows login and service recovery are unsupported on Linux")

func Start(path string) error {
	command := exec.Command(path)
	command.Dir = filepath.Dir(path)
	return StartIndependent(command)
}

func StartAtLogin(string) error          { return ErrUnsupported }
func StartRecovery(string, string) error { return ErrUnsupported }
func StartAfterInstall(string) error     { return ErrUnsupported }
