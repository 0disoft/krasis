//go:build windows

package launchrecord

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupShutdownOnlyAcceptsUnchangedCurrentOrPreviousImage(t *testing.T) {
	base := t.TempDir()
	images := []string{}
	for _, gen := range []int{248, 249} {
		bundle := t.TempDir()
		for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
			if err := os.WriteFile(filepath.Join(bundle, name), []byte("test image"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		ui := filepath.Join(bundle, "krasis.exe")
		if err := Register(base, ui, gen, "test"); err != nil {
			t.Fatal(err)
		}
		images = append(images, ui)
	}
	for _, ui := range images {
		if err := verifyShutdownImage(base, ui); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifyShutdownImage(base, filepath.Join(t.TempDir(), "krasis.exe")); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown image accepted", err)
	}
	if err := os.WriteFile(images[0], []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyShutdownImage(base, images[0]); !errors.Is(err, ErrInvalid) {
		t.Fatal("changed image accepted", err)
	}
}
