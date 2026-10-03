//go:build windows

package inputworker

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInstallationRefreshLockIsReadOnlyAndDetectsExclusiveInstaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "INPUT-TRANSACTION.lock")
	if running, err := installationRefreshRunning(path); err != nil || running {
		t.Fatal(running, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("reader created lock")
	}
	if err := os.WriteFile(path, []byte("owned transaction lock"), 0600); err != nil {
		t.Fatal(err)
	}
	name, _ := windows.UTF16PtrFromString(path)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if running, err := installationRefreshRunning(path); err != nil || !running {
		windows.CloseHandle(handle)
		t.Fatal(running, err)
	}
	windows.CloseHandle(handle)
	if running, err := installationRefreshRunning(path); err != nil || running {
		t.Fatal(running, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "owned transaction lock" {
		t.Fatal("reader changed lock")
	}
}
