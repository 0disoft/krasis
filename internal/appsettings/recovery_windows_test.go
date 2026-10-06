//go:build windows

package appsettings

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryTaskRegistrationIsUserScopedAndReversible(t *testing.T) {
	t.Setenv("KRASIS_RECOVERY_DEBUG", "1")
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	suffix := "-test-" + hex.EncodeToString(nonce[:])
	base := t.TempDir()
	target := filepath.Join(base, "Krasis", "launch", "krasis-cli.exe")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("not executable; task removed before trigger"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := setRecoveryTask(base, suffix, false); err != nil {
			t.Error("temporary task cleanup failed", err)
		}
	})
	for range 2 {
		if err := setRecoveryTask(base, suffix, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := setRecoveryTask(base, suffix, false); err != nil {
		t.Fatal(err)
	}
	if err := setRecoveryTask(base, suffix, false); err != nil {
		t.Fatal("absence not idempotent", err)
	}
}
