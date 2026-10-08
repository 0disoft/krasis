//go:build windows

package appsettings

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestStartupUsesQuotedStableLauncher(t *testing.T) {
	cmd := StartupCommand(`C:\Users\Test User\AppData\Local`)
	if !strings.HasPrefix(cmd, `"C:\Users\Test User\`) || !strings.HasSuffix(cmd, `krasis-cli.exe" relaunch --startup`) {
		t.Fatal(cmd)
	}
}

func TestStartupRegistrationPreservesForeignAndUnreadableValues(t *testing.T) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	path := `Software\Rodisoft\Krasis-Startup-Test-` + hex.EncodeToString(nonce[:])
	key, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { key.Close(); registry.DeleteKey(registry.CURRENT_USER, path) })
	base := t.TempDir()
	foreign := `"C:\Other App\launcher.exe" --startup`
	if err := key.SetStringValue("Krasis", foreign); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		if err := setStartupAt(registry.CURRENT_USER, path, base, enabled); err == nil {
			t.Fatal("foreign startup registration was accepted")
		}
		if value, _, err := key.GetStringValue("Krasis"); err != nil || value != foreign {
			t.Fatal("foreign registration was changed", value, err)
		}
	}
	if _, err := startupEnabledAt(registry.CURRENT_USER, path, base); err == nil {
		t.Fatal("foreign registration was reported as a disabled preference")
	}
	if err := key.SetDWordValue("Krasis", 42); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		if err := setStartupAt(registry.CURRENT_USER, path, base, enabled); err == nil {
			t.Fatal("unreadable registration was accepted")
		}
		if value, _, err := key.GetIntegerValue("Krasis"); err != nil || value != 42 {
			t.Fatal("unreadable registration was changed", value, err)
		}
	}
	if err := key.DeleteValue("Krasis"); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, true, false, false} {
		if err := setStartupAt(registry.CURRENT_USER, path, base, enabled); err != nil {
			t.Fatal("owned/missing registration rejected", err)
		}
		if actual, err := startupEnabledAt(registry.CURRENT_USER, path, base); err != nil || actual != enabled {
			t.Fatal("startup preference did not round trip", actual, err)
		}
	}
}
