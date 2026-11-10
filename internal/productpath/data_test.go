package productpath

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalDataPlatformPolicy(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(base, "windows"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "xdg"))
	t.Setenv("HOME", filepath.Join(base, "home"))
	path, err := LocalData()
	want := filepath.Join(base, "windows")
	if runtime.GOOS == "linux" {
		want = filepath.Join(base, "xdg")
	}
	if err != nil || path != want {
		t.Fatalf("LocalData = %q, %v; want %q", path, err, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("read-only resolution created user data")
	}
}

func TestLinuxLocalDataHomeFallbackAndPreparation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux XDG policy")
	}
	base := t.TempDir()
	t.Setenv("HOME", base)
	t.Setenv("XDG_DATA_HOME", "relative-is-ignored")
	want := filepath.Join(base, ".local", "share")
	if path, err := LocalData(); err != nil || path != want {
		t.Fatalf("fallback = %q, %v", path, err)
	}
	if path, err := PrepareLocalData(); err != nil || path != want {
		t.Fatalf("prepare = %q, %v", path, err)
	}
	info, err := os.Stat(want)
	if err != nil || !info.IsDir() {
		t.Fatalf("prepared parent = %v", err)
	}
	t.Setenv("HOME", "relative")
	if _, err := LocalData(); err == nil {
		t.Fatal("relative HOME accepted")
	}
}
