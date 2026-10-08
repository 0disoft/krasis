//go:build windows

package launchrecord

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/productidentity"
)

func TestRemovalPreservesExternalRecordsInBothLaunchOrders(t *testing.T) {
	for _, portableLast := range []bool{true, false} {
		base, installed, portable := t.TempDir(), t.TempDir(), t.TempDir()
		images := []string{portable, installed}
		if portableLast {
			images = []string{installed, portable}
		}
		for i, image := range images {
			for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
				if err := os.WriteFile(filepath.Join(image, name), []byte("fixture; not executed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := Register(base, filepath.Join(image, "krasis.exe"), i+1, "test"); err != nil {
				t.Fatal(err)
			}
		}
		launch, _ := Directory(base)
		preserved, removed := "previous.json", "current.json"
		if portableLast {
			preserved, removed = removed, preserved
		}
		before, err := os.ReadFile(filepath.Join(launch, preserved))
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateRecordRemoval(base, []string{installed}); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(installed, "krasis.exe")); err != nil {
			t.Fatal(err)
		}
		if err := RemoveRecordsInDirectories(base, []string{installed}); err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(filepath.Join(launch, preserved))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("external launch history changed", err)
		}
		if _, err := os.Stat(filepath.Join(launch, removed)); !os.IsNotExist(err) {
			t.Fatal("installed launch history was retained", err)
		}
	}
}

func TestRecordRemovalRechecksNewLaunchAndRejectsMalformedHistory(t *testing.T) {
	base, installed, portable := t.TempDir(), t.TempDir(), t.TempDir()
	for i, image := range []string{installed, portable} {
		for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
			if err := os.WriteFile(filepath.Join(image, name), []byte("fixture; not executed"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if i == 1 {
			if err := ValidateRecordRemoval(base, []string{installed}); err != nil {
				t.Fatal(err)
			}
		}
		if err := Register(base, filepath.Join(image, "krasis.exe"), i+1, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveRecordsInDirectories(base, []string{installed}); err != nil {
		t.Fatal(err)
	}
	if r, err := Load(base); err != nil || r.Executable != filepath.Join(portable, "krasis.exe") {
		t.Fatal("new launch was deleted after preflight", err)
	}
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := replace(dir, "previous.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	dir.Close()
	if err := ValidateRecordRemoval(base, []string{portable}); err == nil {
		t.Fatal("malformed history accepted by preflight")
	}
	if err := RemoveRecordsInDirectories(base, []string{portable}); err == nil {
		t.Fatal("malformed history accepted by deletion")
	}
	if _, err := Load(base); err != nil {
		t.Fatal("failure deleted the valid current record", err)
	}
}
