//go:build windows && desktop

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLauncherRequiresPackagedControl(t *testing.T) {
	file := filepath.Join(t.TempDir(), "control.exe")
	data := []byte("packaged UI")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(data))
	if err := verifyControl(file, want); err != nil {
		t.Fatal(err)
	}
	if verifyControl(file, "") == nil {
		t.Fatal("accepted unset build identity")
	}
	if err := os.WriteFile(file, []byte("other UI"), 0600); err != nil {
		t.Fatal(err)
	}
	if verifyControl(file, want) == nil {
		t.Fatal("accepted changed UI")
	}
	if verifyControl(filepath.Join(t.TempDir(), "missing.exe"), want) == nil {
		t.Fatal("accepted missing UI")
	}
}
