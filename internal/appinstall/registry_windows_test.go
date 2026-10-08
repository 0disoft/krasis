//go:build windows

package appinstall

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestRemovalRequiresVisibleOwnedUninstallRegistration(t *testing.T) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := `Software\Rodisoft\Krasis-Setup-Test-` + hex.EncodeToString(nonce[:])
	path := t.TempDir()
	if err := validateUninstallAt(registry.CURRENT_USER, name, path); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing registration permits deletion", err)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, name, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registry.DeleteKey(registry.CURRENT_USER, name); err != nil {
			t.Error(err)
		}
	})
	if err = k.SetStringValue("InstallLocation", path); err != nil {
		k.Close()
		t.Fatal(err)
	}
	k.Close()
	if err := validateUninstallAt(registry.CURRENT_USER, name, path); err != nil {
		t.Fatal("owned registration rejected", err)
	}
	if err := validateUninstallAt(registry.CURRENT_USER, name, t.TempDir()); !errors.Is(err, ErrInvalid) {
		t.Fatal("foreign registration permits deletion", err)
	}
}
