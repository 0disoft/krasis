//go:build windows

package launcherui

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/peerupdate"
)

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
