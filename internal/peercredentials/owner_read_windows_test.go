//go:build windows

package peercredentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnerReadScopeDoesNotGrantOrRepairAccess(t *testing.T) {
	owner, err := currentUserSIDString()
	if err != nil {
		t.Fatal(err)
	}
	scope, err := ReadScopeForOwner(owner)
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	if dir, err := scope.OpenDirectory(base); err == nil {
		dir.Close()
		t.Fatal("read scope repaired inherited ACL")
	}
	if _, err := ProtectSecurityDirectory(base); err != nil {
		t.Fatal(err)
	}
	dir, err := scope.OpenDirectory(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	path := filepath.Join(base, "policy.json")
	if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ProtectSecurityFile(path); err != nil {
		t.Fatal(err)
	}
	file, err := dir.Open("policy.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := scope.ValidateFile(file); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("changed")); err == nil {
		t.Fatal("read scope opened writable file")
	}
	if data, _ := os.ReadFile(path); string(data) != "unchanged" {
		t.Fatal("read changed protected data")
	}
	if owner != "S-1-5-18" {
		if _, err := ReadScopeForOwner("S-1-5-21-1-2-3-1001"); err == nil && owner != "S-1-5-21-1-2-3-1001" {
			t.Fatal("ordinary caller selected another owner")
		}
	}
	var empty OwnerReadScope
	if empty.ValidateFile(file) == nil || empty.ValidateDirectory(dir.Handle()) == nil {
		t.Fatal("zero scope accepted")
	}
}
