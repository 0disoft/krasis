//go:build windows

package productpanel

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func LaunchReceivedWorkspaceUpdate(directory, revision, alias string) error {
	return launchReceivedUpdate(directory, revision, alias, localapi.AgentGeneration, func(command *exec.Cmd) error {
		command.Args = append(command.Args, "--wait-pid", strconv.Itoa(os.Getpid()))
		if err := command.Start(); err != nil {
			return err
		}
		return command.Process.Release()
	})
}

func LaunchReceivedUpdate(directory, revision, alias string) error {
	return launchReceivedUpdate(directory, revision, alias, localapi.AgentGeneration, func(command *exec.Cmd) error {
		if err := command.Start(); err != nil {
			return err
		}
		return command.Process.Release()
	})
}

func launchReceivedUpdate(directory, revision, alias string, generation int, start func(*exec.Cmd) error) error {
	parent := filepath.Join(os.Getenv("LOCALAPPDATA"), "Krasis", "updates")
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	if filepath.Dir(absolute) != parent || len(filepath.Base(absolute)) != 32 {
		return errors.New("invalid staged update path")
	}
	if _, err := peercredentials.ValidateSecurityDirectory(parent); err != nil {
		return err
	}
	guard, err := securefs.OpenDirectory(absolute)
	if err != nil {
		return err
	}
	defer guard.Close()
	// Reuse the no-reparse directory handle. Opening an os.Root alongside this
	// DELETE-capable handle fails Windows sharing checks before validation.
	data, err := updatebundle.Pack(guard)
	if err != nil {
		return err
	}
	bundle, err := updatebundle.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if bundle.Metadata().Revision != revision || bundle.Metadata().Generation <= generation {
		return errors.New("stale update")
	}
	command := exec.Command(filepath.Join(absolute, "krasis-control.exe"), "--updated-peer", alias, "--expected-build", revision)
	command.Dir = absolute
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	for _, key := range []string{"SystemRoot", "WINDIR", "LOCALAPPDATA", "TEMP", "TMP", "USERPROFILE"} {
		if value, ok := os.LookupEnv(key); ok {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	return start(command)
}
