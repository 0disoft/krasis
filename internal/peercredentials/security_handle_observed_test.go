package peercredentials

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestObservedFileHandleValidationPreservesVerdictAndDoesNotRepair(t *testing.T) {
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
	var stages []string
	err = ValidateSecurityFileHandleObserved(file, func(stage string) { stages = append(stages, stage) })
	want := []string{"info", "acl_check"}
	if runtime.GOOS == "windows" {
		want = []string{"info", "owner", "acl_query", "acl_check"}
	}
	if err != nil || !slices.Equal(stages, want) || ValidateSecurityFileHandle(file) != nil {
		t.Fatalf("observed file validation: stages=%v error=%v", stages, err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	stages = nil
	if ValidateSecurityFileHandleObserved(file, func(stage string) { stages = append(stages, stage) }) == nil ||
		!slices.Equal(stages, []string{"info"}) {
		t.Fatalf("closed handle continued past failed info query: %v", stages)
	}
	if ValidateSecurityFileHandleObserved(nil, func(string) { t.Fatal("nil handle emitted progress") }) == nil {
		t.Fatal("nil handle accepted")
	}
}
