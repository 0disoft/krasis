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
