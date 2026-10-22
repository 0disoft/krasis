//go:build windows

package appsettings

import (
	"os"

	"golang.org/x/sys/windows"
)

func lockPreferencesFile(file *os.File) (func() error, error) {
	var overlap windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlap); err != nil {
		return nil, err
	}
	return func() error { return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlap) }, nil
}
