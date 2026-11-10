//go:build linux

package peercredentials

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/securefs"
)

func TestLinuxDirectoryHandlePolicyDoesNotSilentlyRepair(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := securefs.OpenReadDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if !errors.Is(ValidateSecurityDirectoryHandle(dir.Handle()), ErrInsecurePrivateKey) {
		t.Fatal("group/world-readable directory accepted")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o755 {
		t.Fatal("validation changed directory mode")
	}
	if err := ProtectSecurityDirectoryHandle(dir.Handle()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecurityDirectoryHandle(dir.Handle()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecurityDirectoryHandle(^uintptr(0)); err == nil {
		t.Fatal("invalid descriptor accepted")
	}
}

func TestLinuxFilePolicyRejectsHardlinksAndSymlinks(t *testing.T) {
	base := t.TempDir()
	if _, err := ProtectSecurityDirectory(base); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "key")
	if err := os.WriteFile(path, []byte("test-policy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ProtectSecurityFile(path); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "symlink")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecurityFile(link); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := os.Link(path, filepath.Join(base, "hardlink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ProtectSecurityFile(path); !errors.Is(err, ErrInsecurePrivateKey) {
		t.Fatalf("hardlink protection error = %v", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o644 {
		t.Fatal("unsafe hardlink target was modified")
	}
	alias := filepath.Join(t.TempDir(), "directory-link")
	if err := os.Symlink(base, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSecurityDirectory(alias); err == nil {
		t.Fatal("symlink directory accepted")
	}
}

func TestLinuxFileHandleSurvivesPathReplacement(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "key")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecurityFileHandle(file); err != nil {
		t.Fatalf("pinned file changed verdict after path replacement: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecurityFileHandle(file); err == nil {
		t.Fatal("closed descriptor accepted")
	}
}

func TestLinuxForeignOwnerIsNeverRepaired(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing file ownership requires root")
	}
	path := filepath.Join(t.TempDir(), "foreign")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := file.Chown(65534, -1); err != nil {
		t.Fatal(err)
	}
	defer file.Chown(0, -1)
	if err := ProtectSecurityFileHandle(file); !errors.Is(err, ErrInsecurePrivateKey) {
		t.Fatalf("foreign owner error = %v", err)
	}
	info, _ := file.Stat()
	if info.Mode().Perm() != 0o644 {
		t.Fatal("foreign file permissions were modified")
	}
}
