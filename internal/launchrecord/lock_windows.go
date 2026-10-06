//go:build windows

package launchrecord

import (
	"errors"
	"os"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows"
)

func lockRegistration(dir *securefs.Directory) (func(), error) {
	f, err := dir.CreateExclusive(".registration.lock", 0600)
	if errors.Is(err, os.ErrExist) {
		f, err = dir.OpenReadWrite(".registration.lock")
	} else if err == nil {
		err = peercredentials.ProtectSecurityFileHandle(f)
	}
	if err != nil {
		if f != nil {
			f.Close()
		}
		return nil, err
	}
	if err = peercredentials.ValidateSecurityFileHandle(f); err != nil {
		f.Close()
		return nil, err
	}
	var overlap windows.Overlapped
	if err = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlap); err != nil {
		f.Close()
		return nil, err
	}
	return func() { windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlap); f.Close() }, nil
}
