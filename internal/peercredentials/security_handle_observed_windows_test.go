//go:build windows

package peercredentials

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/sys/windows"
)

func TestObservedHandleValidationRejectsChangedACLAndHardLinks(t *testing.T) {
	parent := t.TempDir()
	if _, err := ProtectSecurityDirectory(parent); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "policy")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := ProtectSecurityFile(path); err != nil {
		t.Fatal(err)
	}
	// The parent stays private. Give only this empty fixture a noncanonical
	// world ACL; a fresh observed check must reject it without repairing it.
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;WD)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	var stages []string
	if ValidateSecurityFileHandleObserved(file, func(stage string) { stages = append(stages, stage) }) == nil ||
		!slices.Equal(stages, []string{"info", "owner", "acl_query", "acl_check"}) {
		t.Fatalf("changed ACL was accepted or stages lost: %v", stages)
	}
	if ValidateSecurityFileHandle(file) == nil {
		t.Fatal("observer repaired the invalid ACL")
	}
	if err := ProtectSecurityFile(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, path+"-link"); err != nil {
		t.Fatal(err)
	}
	stages = nil
	if ValidateSecurityFileHandleObserved(file, func(stage string) { stages = append(stages, stage) }) == nil ||
		!slices.Equal(stages, []string{"info"}) {
		t.Fatalf("hard-linked file reached ACL validation: %v", stages)
	}
}
