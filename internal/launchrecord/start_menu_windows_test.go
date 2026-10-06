//go:build windows

package launchrecord

import (
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
