//go:build windows

package launchrecord

import (
	"bytes"
	"encoding/json"
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

func TestStartMenuRemovalPreflightRejectsChangedAndMalformedReceiptsWithoutWriting(t *testing.T) {
	base := t.TempDir()
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	target, link := filepath.Join(base, "krasis-cli.exe"), filepath.Join(t.TempDir(), "Krasis.lnk")
	if r, exists, err := startMenuRemoval(dir, target, link); err != nil || r != nil || exists {
		t.Fatal("missing registration rejected", err)
	}
	linkBytes := []byte("shortcut fixture; never executed")
	if err := os.WriteFile(link, linkBytes, 0600); err != nil {
		t.Fatal(err)
	}
	hash, err := linkDigest(link)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(menuReceipt{target, link, hash})
	if err := replace(dir, "start-menu.json", body); err != nil {
		t.Fatal(err)
	}
	if r, exists, err := startMenuRemoval(dir, target, link); err != nil || r == nil || !exists {
		t.Fatal("owned shortcut rejected", err)
	}
	for _, invalid := range [][]byte{[]byte("{}"), append(append([]byte{}, body...), []byte(" {}")...), []byte(`{"Target":"foreign"}`), bytes.Replace(body, []byte(`"SHA256"`), []byte(`"unknown"`), 1)} {
		if err := replace(dir, "start-menu.json", invalid); err != nil {
			t.Fatal(err)
		}
		if _, _, err := startMenuRemoval(dir, target, link); err == nil {
			t.Fatal("malformed receipt accepted")
		}
		path, _ := Directory(base)
		if after, err := os.ReadFile(filepath.Join(path, "start-menu.json")); err != nil || !bytes.Equal(after, invalid) {
			t.Fatal("preflight changed invalid receipt", err)
		}
	}
	if err := replace(dir, "start-menu.json", body); err != nil {
		t.Fatal(err)
	}
	if _, _, err := startMenuRemoval(dir, target+"-foreign", link); err == nil {
		t.Fatal("foreign target accepted")
	}
	if err := os.WriteFile(link, []byte("changed shortcut"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := startMenuRemoval(dir, target, link); err == nil {
		t.Fatal("changed shortcut accepted")
	}
	if after, err := os.ReadFile(link); err != nil || string(after) != "changed shortcut" {
		t.Fatal("preflight changed foreign link", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if r, exists, err := startMenuRemoval(dir, target, link); err != nil || r == nil || exists {
		t.Fatal("missing owned shortcut rejected", err)
	}
	path, _ := Directory(base)
	if after, err := os.ReadFile(filepath.Join(path, "start-menu.json")); err != nil || !bytes.Equal(after, body) {
		t.Fatal("preflight removed missing link receipt", err)
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
