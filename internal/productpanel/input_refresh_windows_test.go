//go:build windows

package productpanel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestInputRefreshPinsRejectReplacementAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.exe")
	if err := os.WriteFile(path, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := pinRefreshFile(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err == nil {
		t.Fatal("writable pinned image")
	}
	if err := os.Remove(path); err == nil {
		t.Fatal("replaceable pinned image")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal("pin leaked", err)
	}
}

func TestInputRefreshRejectsIncompleteBundleBeforeConsent(t *testing.T) {
	if runInputInstaller(context.Background(), t.TempDir(), "a123") != ErrInputRefreshFailed {
		t.Fatal("invalid revision accepted")
	}
	if runInputInstaller(context.Background(), t.TempDir(), strings.Repeat("a", 40)) != ErrInputRefreshFailed {
		t.Fatal("missing checksums accepted")
	}
}

func TestInputRefreshShellExecuteLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) == 8 && (unsafe.Sizeof(shellExecuteInfo{}) != 112 || unsafe.Offsetof(shellExecuteInfo{}.Process) != 104) {
		t.Fatal("invalid Win32 x64 layout")
	}
}
