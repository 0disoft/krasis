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
	if got := updateBundleDirectory(filepath.Join(base, "krasis.exe")); got != filepath.Join(base, "peer-update") {
		t.Fatal(got)
	}
	if err := os.WriteFile(filepath.Join(base, "BUILD.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := updateBundleDirectory(filepath.Join(base, "krasis.exe")); got != base {
		t.Fatal(got)
	}
	if got := transitionBundleDirectory(filepath.Join(base, "krasis-control.exe")); got != "" {
		t.Fatal(got)
	}
	if got := transitionBundleDirectory(filepath.Join(base, "g63", "krasis.exe")); got != "" {
		t.Fatal("missing transition bundle was offered")
	}
	if err := os.MkdirAll(filepath.Join(base, "g75", "peer-update"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "g75", "peer-update", "BUILD.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := transitionBundleDirectory(filepath.Join(base, "g77", "krasis.exe")); got != filepath.Join(base, "g75", "peer-update") {
		t.Fatal(got)
	}
	if got := transitionBundleDirectory(filepath.Join(base, "g64", "peer-update", "krasis-control.exe")); got != filepath.Join(base, "g75", "peer-update") {
		t.Fatal(got)
	}
	if err := waitUpdateParent(uint32(os.Getpid())); err == nil {
		t.Fatal("waited on self")
	}
	if err := waitUpdateParent(0); err != nil {
		t.Fatal(err)
	}
}
