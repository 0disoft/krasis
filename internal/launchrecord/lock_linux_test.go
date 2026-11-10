//go:build linux

package launchrecord

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

func TestLinuxRegistrationOwnershipAndUnsafeLock(t *testing.T) {
	base := t.TempDir()
	if _, err := peercredentials.ProtectSecurityDirectory(base); err != nil {
		t.Fatal(err)
	}
	dir, err := securefs.OpenDirectory(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	unlock, err := lockRegistration(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := lockRegistration(dir); err == nil {
		second()
		t.Fatal("simultaneous registration accepted")
	}
	unlock()
	unlock, err = lockRegistration(dir)
	if err != nil {
		t.Fatalf("registration lock not released: %v", err)
	}
	unlock()
	if err := os.Remove(filepath.Join(base, ".registration.lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "foreign"), filepath.Join(base, ".registration.lock")); err != nil {
		t.Fatal(err)
	}
	if unlock, err := lockRegistration(dir); err == nil {
		unlock()
		t.Fatal("symlink registration lock accepted")
	}
}
