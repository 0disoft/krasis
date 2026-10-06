//go:build windows

package appsettings

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSettingsReadableWhileRecoveryDirectoryIsInUse(t *testing.T) {
	base := t.TempDir()
	want := Settings{Schema: Schema, StartAtLogin: true, AutoUpdate: true}
	if err := Save(base, want); err != nil {
		t.Fatal(err)
	}
	peer := Resume{Alias: "B", Fingerprint: strings.Repeat("a", 64)}
	if err := SaveResume(base, peer); err != nil {
		t.Fatal(err)
	}
	if err := PauseRecovery(base, false); err != nil {
		t.Fatal(err)
	}
	path, err := syscall.UTF16PtrFromString(filepath.Join(base, "Krasis", "launch"))
	if err != nil {
		t.Fatal(err)
	}
	// A scheduled CLI uses this as its working directory; that handle does not
	// grant delete sharing. Reading preferences must not request DELETE access.
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.CloseHandle(handle) })
	if got, err := Load(base); err != nil || got != want {
		t.Fatalf("settings read: %+v %v", got, err)
	}
	if got, err := LoadResume(base); err != nil || got != peer {
		t.Fatalf("resume read: %+v %v", got, err)
	}
	if paused, err := RecoveryPaused(base); err != nil || paused {
		t.Fatalf("recovery read: %v %v", paused, err)
	}
}
