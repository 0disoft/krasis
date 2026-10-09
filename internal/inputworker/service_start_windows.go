//go:build windows

package inputworker

import (
	"context"
	"io"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func checkServiceStart(root string, images installImages, args []string) error {
	parent, err := openProtected(root, true)
	if err != nil {
		return err
	}
	defer parent.Close()
	file, err := openProtected(filepath.Join(root, "INPUT-TRANSACTION.json"), false)
	var journal *installJournal
	if err != nil {
		if !missingInstallPath(filepath.Join(root, "INPUT-TRANSACTION.json")) {
			return err
		}
		for _, name := range []string{"INPUT-TRANSACTION.pending", "Input.stage", "Input.rollback", "Input.retired"} {
			if !missingInstallPath(filepath.Join(root, name)) {
				return ErrRecovery
			}
		}
		if busy, err := installationRefreshRunning(filepath.Join(root, "INPUT-TRANSACTION.lock")); err != nil || busy {
			return ErrRecovery
		}
	} else {
		data, readErr := io.ReadAll(io.LimitReader(file, 2049))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return ErrRecovery
		}
		value, err := decodeInstallJournal(data)
		if err != nil {
			return err
		}
		journal = &value
	}
	if !serviceStartAllowed(args, images, journal) {
		return ErrRecovery
	}
	return nil
}

func (manager *installSCM) enableAutomatic(ctx context.Context) error {
	handle, err := manager.open(ctx, windows.SERVICE_CHANGE_CONFIG)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(handle)
	if err := windows.ChangeServiceConfig(handle, windows.SERVICE_NO_CHANGE, windows.SERVICE_AUTO_START, windows.SERVICE_NO_CHANGE, nil, nil, nil, nil, nil, nil, nil); err != nil {
		return installNativeError("service_config", ErrAuthority, err)
	}
	config, err := readInstallServiceConfig(handle)
	if err != nil {
		return err
	}
	if !validInstallServiceConfig(config, manager.command()) || config.start != windows.SERVICE_AUTO_START {
		return ErrAuthority
	}
	return nil
}
