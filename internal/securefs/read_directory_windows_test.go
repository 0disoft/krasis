//go:build windows

package securefs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadDirectoryWorksWithoutDeleteSharing(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "image.exe"), []byte("MZ-test"), 0600); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// Hold the same sharing constraint as a live working-directory handle.
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	if directory, err := OpenDirectory(path); !errors.Is(err, syscall.Errno(32)) {
		if directory != nil {
			directory.Close()
		}
		t.Fatalf("delete-sharing reproduction: %v", err)
	}
	directory, err := OpenReadDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	file, err := directory.Open("image.exe")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(data) != "MZ-test" {
		t.Fatal("image read failed", err)
	}
	if err := os.Link(filepath.Join(path, "image.exe"), filepath.Join(path, "alias.exe")); err != nil {
		t.Fatal(err)
	}
	if file, err := directory.Open("image.exe"); !errors.Is(err, ErrHardLink) {
		if file != nil {
			file.Close()
		}
		t.Fatal("read directory bypassed hard-link check", err)
	}
}
