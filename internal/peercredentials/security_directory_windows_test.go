//go:build windows

package peercredentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryHandleValidationRejectsUnsafeACLAndReparseAncestors(t *testing.T) {
	root := t.TempDir()
	if _, err := ValidateSecurityDirectory(root); err == nil {
		t.Fatal("accepted an unprotected inherited ACL")
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ProtectSecurityDirectory(child); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSecurityDirectory(child); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := ValidateSecurityDirectory(filepath.Join(link, "child")); err == nil {
		t.Fatal("accepted a reparse ancestor leading to an otherwise secure directory")
	}
}

func TestOpenProtectedDirectoryChecksOpenedHandle(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "protected")
	if err := os.Mkdir(protected, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ProtectSecurityDirectory(protected); err != nil {
		t.Fatal(err)
	}
	for _, readOnly := range []bool{false, true} {
		dir, err := OpenProtectedDirectory(protected, readOnly)
		if err != nil {
			t.Fatal(err)
		}
		if err := dir.Close(); err != nil {
			t.Fatal(err)
		}
		if dir, err := OpenProtectedDirectory(root, readOnly); err == nil {
			_ = dir.Close()
			t.Fatal("accepted directory with inherited unsafe ACL")
		}
	}
}
