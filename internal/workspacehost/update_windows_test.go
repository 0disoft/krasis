//go:build windows && desktop

package workspacehost

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateEntryPointsUseOwnedBundleDirectory(t *testing.T) {
	base := t.TempDir()
	if got := updateBundleDirectory(filepath.Join(base, "krasis-control.exe")); got != base {
		t.Fatal(got)
	}
	if got := updateBundleDirectory(filepath.Join(base, "krasis-workspace.exe")); got != filepath.Join(base, "peer-update") {
		t.Fatal(got)
	}
	if err := waitUpdateParent(uint32(os.Getpid())); err == nil {
		t.Fatal("waited on self")
	}
	if err := waitUpdateParent(0); err != nil {
		t.Fatal(err)
	}
}
