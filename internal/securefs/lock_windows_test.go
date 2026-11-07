//go:build windows

package securefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreatedLockPinsNameUntilEveryHandleCloses(t *testing.T) {
	path := t.TempDir()
	dir, err := OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	first, err := dir.CreateLock("lock", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := dir.OpenLock("lock", true)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	lockPath := filepath.Join(path, "lock")
	checkPinned := func() {
		t.Helper()
		if err := os.Remove(lockPath); err == nil {
			t.Fatal("lock could be deleted while open")
		}
		if err := os.Rename(lockPath, lockPath+".moved"); err == nil {
			t.Fatal("lock could be renamed while open")
		}
		replacement, err := dir.CreateExclusive("replacement", 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := replacement.Close(); err != nil {
			t.Fatal(err)
		}
		if err := dir.Replace("replacement", "lock"); err == nil {
			t.Fatal("lock could be replaced while open")
		}
		if err := dir.Remove("replacement"); err != nil {
			t.Fatal(err)
		}
	}
	checkPinned()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	checkPinned()
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("released lock could not be removed: %v", err)
	}
}
