//go:build windows

package inputworker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// Reproduce the journal rename with exactly the installer's directory pin and
// closed source writer, without elevation, SCM or changing any security policy.
func TestInstallJournalMoveWithPinnedRoot(t *testing.T) {
	root := t.TempDir()
	name, err := windows.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.READ_CONTROL, protectedShareMode(true), nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	pending := filepath.Join(root, "INPUT-TRANSACTION.pending")
	destination := filepath.Join(root, "INPUT-TRANSACTION.json")
	for _, value := range []string{"stage", "replace", "rollback_stop"} {
		if err := os.WriteFile(pending, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		if err := moveInstallPath(pending, destination, true); err != nil {
			op, code, known := installErrorFields(err)
			t.Fatalf("closed journal writer could not move: operation=%s win32=%d known=%t", op, code, known)
		}
		data, err := os.ReadFile(destination)
		if err != nil || string(data) != value {
			t.Fatal("journal replacement not persisted")
		}
	}
	if err := moveInstallPath(root, root+".moved", false); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("directory identity could be replaced while pinned")
	}
	// The same directory pin is used while publishing service receipts and
	// cleaning children; no sharing violation may prevent either operation.
	name, err = windows.UTF16PtrFromString(destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.DeleteFile(name); err != nil {
		t.Fatal("directory pin prevented child cleanup")
	}
}

func TestProtectedFilePinStillBlocksImageMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.exe")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.READ_CONTROL, protectedShareMode(false), nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err := os.WriteFile(path, []byte("changed"), 0600); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("file pin permitted image write")
	}
	if err := moveInstallPath(path, path+".moved", false); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatal("file pin permitted image replacement")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original" {
		t.Fatal("pinned image changed")
	}
}
