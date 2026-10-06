//go:build windows

package launchrecord

import (
	"github.com/0disoft/krasis/internal/productidentity"
	"os"
	"path/filepath"
	"testing"
)

func TestStartMenuShortcutUsesFixedCLIAndPreservesForeignLink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "krasis-cli.exe")
	link := filepath.Join(root, "Krasis.lnk")
	if err := os.WriteFile(target, []byte("MZ-test"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := registerStartMenuLink(target, link); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := registerStartMenuLink(target, link); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(link)
	if err != nil || string(before) != string(after) {
		t.Fatal("existing owned shortcut changed", err)
	}
	other := filepath.Join(t.TempDir(), "krasis-cli.exe")
	if err := os.WriteFile(other, []byte("MZ-other"), 0600); err != nil {
		t.Fatal(err)
	}
	if registerStartMenuLink(other, link) == nil {
		t.Fatal("foreign shortcut overwritten")
	}
	after, err = os.ReadFile(link)
	if err != nil || string(before) != string(after) {
		t.Fatal("foreign shortcut not preserved")
	}
}

func TestStartMenuReceiptSkipsHelperUntilShortcutChanges(t *testing.T) {
	base := t.TempDir()
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	target, link := filepath.Join(base, "krasis-cli.exe"), filepath.Join(t.TempDir(), "Krasis.lnk")
	calls := 0
	register := func(string, string) error { calls++; return os.WriteFile(link, []byte("verified link"), 0600) }
	for range 2 {
		if err := ensureStartMenuLink(dir, target, link, register); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("helper repeated", calls)
	}
	if err := os.WriteFile(link, []byte("changed link"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureStartMenuLink(dir, target, link, register); err != nil || calls != 2 {
		t.Fatal("change not rechecked", err)
	}
}
