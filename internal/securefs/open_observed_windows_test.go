//go:build windows

package securefs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestObservedOpenPreservesChecksAndHandleIdentity(t *testing.T) {
	path := t.TempDir()
	name := filepath.Join(path, "policy")
	if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenReadDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	var stages []string
	file, err := directory.OpenObserved("policy", func(stage string) {
		stages = append(stages, stage)
		if stage == "info" {
			// The native handle is already open. Metadata checks and the Go
			// wrapper must stay on it even if the path is replaced now.
			if err := os.Rename(name, name+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, []byte("replacement"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(data) != "original" || !slices.Equal(stages, []string{"native", "info", "wrap"}) {
		t.Fatal("observed open lost identity or stages", string(data), stages, err)
	}
	if err := os.Link(name, name+"-link"); err != nil {
		t.Fatal(err)
	}
	stages = nil
	file, err = directory.OpenObserved("policy", func(stage string) { stages = append(stages, stage) })
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, ErrHardLink) || !slices.Equal(stages, []string{"native", "info"}) {
		t.Fatal("observed open bypassed hard-link rejection", stages, err)
	}
	stages = nil
	file, err = directory.OpenObserved("../policy", func(stage string) { stages = append(stages, stage) })
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, ErrUnsafePath) || len(stages) != 0 {
		t.Fatal("unsafe name reached native open", stages, err)
	}
}

func TestObservedOpenRejectsReparseWithoutWrappingHandle(t *testing.T) {
	path := t.TempDir()
	target := filepath.Join(path, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(path, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	directory, err := OpenReadDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	var stages []string
	file, err := directory.OpenObserved("link", func(stage string) { stages = append(stages, stage) })
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, ErrUnsafePath) || slices.Contains(stages, "wrap") {
		t.Fatal("reparse reached Go file wrapping", stages, err)
	}
}
