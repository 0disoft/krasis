package securefs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirectoryCreateAndPromoteNoReplace(t *testing.T) {
	path := t.TempDir()
	directory, err := OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	file, name, err := directory.CreateTemporary(".secure-", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAndSync(file, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := directory.PromoteNoReplace(name, "result.json"); err != nil {
		t.Fatal(err)
	}
	second, secondName, err := directory.CreateTemporary(".secure-", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	defer directory.Remove(secondName)
	if err := directory.PromoteNoReplace(secondName, "result.json"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("PromoteNoReplace error = %v, want existence error", err)
	}
}

func TestDirectoryHandleSurvivesParentPathReplacement(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows handle-relative behavior")
	}
	root := t.TempDir()
	original := filepath.Join(root, "owned")
	moved := filepath.Join(root, "moved")
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	directory, err := OpenDirectory(original)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := directory.CreateExclusive("bound.txt", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moved, "bound.txt")); err != nil {
		t.Fatalf("opened directory did not retain original identity: %v", err)
	}
	if _, err := os.Stat(filepath.Join(original, "bound.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement path received secure write: %v", err)
	}
}

func TestDirectoryRejectsHardLinkedRenameSource(t *testing.T) {
	path := t.TempDir()
	directory, err := OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	file, err := directory.CreateExclusive("source.tmp", 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(path, "source.tmp"), filepath.Join(path, "alias.tmp")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := directory.Replace("source.tmp", "target.json"); !errors.Is(err, ErrHardLink) {
		t.Fatalf("Replace error = %v, want ErrHardLink", err)
	}
}

func TestOpenDirectoryRejectsReparseTraversal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows reparse behavior")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "link")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	if directory, err := OpenDirectory(link); err == nil {
		_ = directory.Close()
		t.Fatal("OpenDirectory accepted reparse traversal")
	}
}
