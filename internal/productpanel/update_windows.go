//go:build windows

package productpanel

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func LaunchReceivedUpdate(directory, revision, alias string) error {
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
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return err
	}
	defer root.Close()
	data, err := updatebundle.Pack(root)
	if err != nil {
		return err
	}
	bundle, err := updatebundle.Read(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if bundle.Metadata().Revision != revision || bundle.Metadata().Generation <= localapi.AgentGeneration {
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
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
