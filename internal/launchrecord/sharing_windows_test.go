//go:build windows

package launchrecord

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func holdWithoutDeleteSharing(t *testing.T, path string) {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.CloseHandle(handle) })
}

func TestRecoveryWorksWhileSourceAndRecordDirectoriesAreInUse(t *testing.T) {
	base, bundle := t.TempDir(), t.TempDir()
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte("MZ-test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	holdWithoutDeleteSharing(t, bundle)
	self := filepath.Join(bundle, "krasis.exe")
	if err := Register(base, self, 136, strings.Repeat("a", 40)); err != nil {
		t.Fatal("live bundle registration failed", err)
	}
	directory, err := Directory(base)
	if err != nil {
		t.Fatal(err)
	}
	holdWithoutDeleteSharing(t, directory)
	if record, err := Inspect(base); err != nil || record.Generation != 136 {
		t.Fatal("live record inspection failed", err)
	}
	launched := false
	if _, err := Relaunch(base, "", func(path string) error {
		if path != self {
			t.Fatal("wrong image")
		}
		launched = true
		return nil
	}); err != nil || !launched {
		t.Fatal("live record relaunch verification failed", err)
	}
}
