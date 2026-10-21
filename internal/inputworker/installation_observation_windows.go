//go:build windows

package inputworker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// ObserveInstallation is for short-lived health/build reads and launch pinning.
// Service/worker owners use LoadInstallation: the installer must stop those
// owners after acquiring its exclusive transaction lock.
func ObserveInstallation() (*Installation, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	return observeInstallation(filepath.Join(root, "Krasis"), LoadInstallation)
}

func observeInstallation(root string, load func() (*Installation, error)) (*Installation, error) {
	path, err := windows.UTF16PtrFromString(filepath.Join(root, "INPUT-TRANSACTION.lock"))
	if err != nil {
		return nil, ErrAuthority
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, ErrAuthority
	}
	guard := os.NewFile(uintptr(handle), "installation-observation")
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 || info.NumberOfLinks != 1 {
		_ = guard.Close()
		return nil, ErrAuthority
	}
	installation, err := load()
	if err != nil || installation == nil {
		_ = guard.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrAuthority
	}
	installation.observation = guard // Closed after all image/metadata pins.
	return installation, nil
}

// Wait only for short-lived readers, never queue behind another installer.
// The exclusive native open decides admission; the observation only classifies
// contention. No transaction, SCM mutation or directory move has begun yet.
func waitInstallWrite(ctx context.Context, path string, open func(string) (*os.File, error)) (*os.File, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		file, err := open(path)
		if err == nil || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			return file, err
		}
		if running, checkErr := installationRefreshRunning(path); checkErr != nil || running {
			return nil, err
		}
		timer := time.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
