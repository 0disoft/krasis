package launchrecord

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectChecksImageWithoutChangingLaunchRecord(t *testing.T) {
	base, bundle := t.TempDir(), t.TempDir()
	self := filepath.Join(bundle, "krasis.exe")
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte("MZ-test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Register(base, self, 119, "test"); err != nil {
		t.Fatal(err)
	}
	dir, _ := Directory(base)
	before, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	if record, err := Inspect(base); err != nil || record.Generation != 119 {
		t.Fatal("valid inspection failed")
	}
	after, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("inspection rewrote launch metadata")
	}
	if err := os.WriteFile(self, []byte("MZ-changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(base); !errors.Is(err, ErrInvalid) {
		t.Fatal("changed executable was accepted")
	}
}

func TestRelaunchRemembersLastUIAndRejectsChangedImage(t *testing.T) {
	base, bundle := t.TempDir(), t.TempDir()
	self := filepath.Join(bundle, "krasis.exe")
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte("MZ-test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Register(base, self, 109, "test"); err != nil {
		t.Fatal(err)
	}
	if err := Register(base, self, 109, "test"); err != nil {
		t.Fatal(err)
	}
	count := 0
	start := func(path string) error {
		count++
		if path != self {
			t.Fatal("wrong image")
		}
		return nil
	}
	record, err := Relaunch(base, "", start)
	if err != nil || count != 1 || record.Generation != 109 {
		t.Fatalf("relaunch: %v, count=%d", err, count)
	}
	path, _ := Directory(base)
	if _, err := os.Stat(filepath.Join(path, "krasis-cli.exe")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(path, "Krasis.cmd")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(self, []byte("MZ-changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Relaunch(base, self, start); !errors.Is(err, ErrInvalid) || count != 1 {
		t.Fatalf("changed image ran: %v", err)
	}
}

func TestRelaunchOnlyUsesFallbackWhenRecordMissing(t *testing.T) {
	base, bundle := t.TempDir(), t.TempDir()
	self := filepath.Join(bundle, "krasis.exe")
	if err := os.WriteFile(self, []byte("MZ-test"), 0600); err != nil {
		t.Fatal(err)
	}
	count := 0
	start := func(string) error { count++; return nil }
	if _, err := Relaunch(base, self, start); err != nil || count != 1 {
		t.Fatal(err)
	}
	if _, err := Relaunch(base, filepath.Join(bundle, "other.exe"), start); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
