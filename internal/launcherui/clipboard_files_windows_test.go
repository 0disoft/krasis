//go:build windows

package launcherui

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peerupdate"
)

func TestClipboardCleanupKeepsReferencedFilesAndFolders(t *testing.T) {
	for _, nested := range []bool{false, true} {
		base := t.TempDir()
		active, stale, recent := filepath.Join(base, "copy-active"), filepath.Join(base, "copy-stale"), filepath.Join(base, "copy-recent")
		for _, stage := range []string{active, stale, recent} {
			if err := os.Mkdir(stage, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stage, "note.txt"), []byte("paste"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		old := time.Now().Add(-48 * time.Hour)
		for _, stage := range []string{active, stale} {
			if err := os.Chtimes(stage, old, old); err != nil {
				t.Fatal(err)
			}
		}
		reference := active
		if nested {
			reference = filepath.Join(active, "note.txt")
		}
		cleanupClipboardFilesWithReferences(base, []string{strings.ToUpper(reference)})
		if _, err := os.Stat(filepath.Join(active, "note.txt")); err != nil {
			t.Fatal("clipboard reference lost", err)
		}
		if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("stale stage retained", err)
		}
		if _, err := os.Stat(recent); err != nil {
			t.Fatal("recent stage lost", err)
		}
		cleanupClipboardFilesWithReferences(base, nil)
		if _, err := os.Stat(active); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("released stage retained", err)
		}
	}
}

func TestClipboardCleanupDoesNotGuessAmbiguousReferences(t *testing.T) {
	base := t.TempDir()
	stage := filepath.Join(base, "copy-old")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stage, old, old); err != nil {
		t.Fatal(err)
	}
	cleanupClipboardFilesWithReferences(base, []string{"relative.txt"})
	if _, err := os.Stat(stage); err != nil {
		t.Fatal("unknown reference pruned", err)
	}
	cleanupClipboardFilesWithReferences(base, []string{stage + "-other"})
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("sibling prefix protected unrelated stage", err)
	}
}

func TestClipboardFileLimitReports(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "oversize.bin"))
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(peerupdate.MaxClipboardFileBytes + 1)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	_, err = packClipboardFiles([]string{file.Name()})
	if !errors.Is(err, errClipboardFileSize) || clipboardFileFailure(err) != "size_limit" {
		t.Fatalf("wrong size report: %v", err)
	}
	_, err = packClipboardFiles(make([]string, 65))
	if !errors.Is(err, errClipboardFileCount) || clipboardFileFailure(err) != "item_limit" {
		t.Fatalf("wrong count report: %v", err)
	}
}

func TestClipboardFileAndFolderRoundTrip(t *testing.T) {
	source := t.TempDir()
	folder := filepath.Join(source, "자료")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(folder, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("copied file contents")
	if err := os.WriteFile(filepath.Join(folder, "note.txt"), content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "other.txt"), []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	packed, err := packClipboardFiles([]string{folder, filepath.Join(source, "other.txt")})
	if err != nil || !peerupdate.ValidClipboardFiles(packed) {
		t.Fatal("pack files", err)
	}
	stage, roots, err := extractClipboardFilesAt(t.TempDir(), packed)
	if err != nil {
		t.Fatal("extract files", err)
	}
	defer os.RemoveAll(stage)
	if len(roots) != 2 || roots[0] != filepath.Join(stage, "자료") || roots[1] != filepath.Join(stage, "other.txt") {
		t.Fatalf("wrong paste roots: %q", roots)
	}
	got, err := os.ReadFile(filepath.Join(stage, "자료", "note.txt"))
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("file contents changed", err)
	}
	if info, err := os.Stat(filepath.Join(stage, "자료", "empty")); err != nil || !info.IsDir() {
		t.Fatal("empty folder missing", err)
	}
	drop, err := hdropBytes(roots)
	if err != nil || binary.LittleEndian.Uint32(drop[:4]) != 20 || binary.LittleEndian.Uint32(drop[16:20]) != 1 || !bytes.HasSuffix(drop, []byte{0, 0, 0, 0}) {
		t.Fatal("invalid CF_HDROP", err)
	}
}

func TestClipboard47MiBFileRoundTrip(t *testing.T) {
	source := filepath.Join(t.TempDir(), "large.bin")
	content := bytes.Repeat([]byte("a"), 47<<20)
	if err := os.WriteFile(source, content, 0600); err != nil {
		t.Fatal(err)
	}
	packed, err := packClipboardFiles([]string{source})
	if err != nil {
		t.Fatal("pack 47 MiB file", err)
	}
	stage, paths, err := extractClipboardFilesAt(t.TempDir(), packed)
	if err != nil {
		t.Fatal("extract 47 MiB file", err)
	}
	defer os.RemoveAll(stage)
	got, err := os.ReadFile(paths[0])
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("large file changed", err)
	}
}
