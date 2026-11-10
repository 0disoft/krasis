//go:build linux

package evidence

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxOutputDirectoryRejectsSymlinkAncestors(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, filepath.Join(link, "child")} {
		if err := validateOutputDirectory(path); !errors.Is(err, ErrUnsafeOutputPath) {
			t.Fatalf("validateOutputDirectory(%q): %v", path, err)
		}
	}
	if err := validateOutputDirectory(filepath.Join(target, "child")); err != nil {
		t.Fatal(err)
	}
}
