package peercredentials

import (
	"runtime"
	"slices"
	"testing"
)

func TestObservedDirectoryValidationPreservesVerdict(t *testing.T) {
	directory := t.TempDir()
	if _, err := ProtectSecurityDirectory(directory); err != nil {
		t.Fatal(err)
	}
	wantPath, err := ValidateSecurityDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	var stages []string
	path, err := ValidateSecurityDirectoryObserved(directory, func(stage string) {
		stages = append(stages, stage)
	})
	if err != nil || path != wantPath {
		t.Fatalf("observed validation changed verdict: %q, %v", path, err)
	}
	wantStages := []string{"stat"}
	if runtime.GOOS == "windows" {
		wantStages = []string{"reparse", "acl_query", "acl_check"}
	}
	if !slices.Equal(stages, wantStages) {
		t.Fatalf("fixed validation stages: %v", stages)
	}
}
