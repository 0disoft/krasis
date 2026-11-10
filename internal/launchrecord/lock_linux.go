//go:build linux

package launchrecord

import (
	"errors"
	"os"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/unix"
)

func lockRegistration(dir *securefs.Directory) (func(), error) {
	if dir == nil {
		return nil, ErrInvalid
	}
	if err := peercredentials.ValidateSecurityDirectoryHandle(dir.Handle()); err != nil {
		return nil, err
	}
	file, err := dir.CreateExclusive(".registration.lock", 0o600)
	if errors.Is(err, os.ErrExist) {
		file, err = dir.OpenReadWrite(".registration.lock")
	}
	if err != nil {
		return nil, err
	}
	if err := peercredentials.ValidateSecurityFileHandle(file); err != nil {
		file.Close()
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return nil, err
	}
	// Keep the inode in place; close releases ownership even after process exit.
	return func() { file.Close() }, nil
}
