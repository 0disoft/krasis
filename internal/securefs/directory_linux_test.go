//go:build linux

package securefs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestLinuxRejectsSymlinkComponentsAndUnsafeLeaves(t *testing.T) {
	path := t.TempDir()
	if err := os.Mkdir(filepath.Join(path, "target"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(path, "target"), filepath.Join(path, "link")); err != nil {
		t.Fatal(err)
	}
	for _, open := range []func(string) (*Directory, error){OpenDirectory, OpenReadDirectory} {
		if dir, err := open(filepath.Join(path, "link", "child")); !errors.Is(err, ErrUnsafePath) {
			if dir != nil {
				dir.Close()
			}
			t.Fatalf("symlink ancestor: %v", err)
		}
	}
	dir, err := OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	for _, name := range []string{"", ".", "..", "../escape", "a/b", `a\b`, "a:b", "\x00"} {
		if file, err := dir.Open(name); !errors.Is(err, ErrUnsafePath) {
			if file != nil {
				file.Close()
			}
			t.Errorf("Open(%q): %v", name, err)
		}
		if err := dir.Remove(name); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("Remove(%q): %v", name, err)
		}
		if err := dir.Replace("source", name); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("Replace(%q): %v", name, err)
		}
	}
	if file, err := dir.Open("link"); !errors.Is(err, ErrUnsafePath) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("symlink leaf: %v", err)
	}
	if err := dir.Remove("link"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("remove symlink: %v", err)
	}
	if err := unix.Mkfifo(filepath.Join(path, "fifo"), 0600); err != nil {
		t.Fatal(err)
	}
	if file, err := dir.Open("fifo"); !errors.Is(err, ErrUnsafePath) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("FIFO: %v", err)
	}
	if file, err := dir.Open("target"); !errors.Is(err, ErrUnsafePath) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("directory as file: %v", err)
	}
}

func TestLinuxObservedOpenKeepsIdentityAndRejectsHardLinks(t *testing.T) {
	path := t.TempDir()
	name := filepath.Join(path, "file")
	if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	dir, err := OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	var stages []string
	file, err := dir.OpenObserved("file", func(stage string) {
		stages = append(stages, stage)
		if stage == "info" {
			if err := os.Rename(name, name+"-old"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, []byte("new"), 0600); err != nil {
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
		t.Fatal(string(data), stages, err)
	}
	if err := os.Link(name, name+"-alias"); err != nil {
		t.Fatal(err)
	}
	for _, open := range []func(string) (*os.File, error){dir.Open, dir.OpenReadWrite, func(name string) (*os.File, error) { return dir.OpenLock(name, true) }} {
		file, err := open("file")
		if file != nil {
			file.Close()
		}
		if !errors.Is(err, ErrHardLink) {
			t.Fatalf("hard link accepted: %v", err)
		}
	}
	if err := dir.Remove("file"); !errors.Is(err, ErrHardLink) {
		t.Fatal(err)
	}
}

func TestLinuxDirectoryClassifiesMissingAndNonDirectoryComponents(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, open := range []func(string) (*Directory, error){OpenDirectory, OpenReadDirectory} {
		for _, path := range []string{filepath.Join(root, "missing"), filepath.Join(root, "missing", "child")} {
			dir, err := open(path)
			if dir != nil {
				dir.Close()
			}
			if !errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrUnsafePath) {
				t.Errorf("missing path %q: %v", path, err)
			}
		}
		for _, path := range []string{filepath.Join(root, "file"), filepath.Join(root, "file", "child")} {
			dir, err := open(path)
			if dir != nil {
				dir.Close()
			}
			if !errors.Is(err, ErrUnsafePath) {
				t.Errorf("non-directory path %q: %v", path, err)
			}
		}
	}
}

func TestLinuxListAndRemoveSelfUsePinnedDirectories(t *testing.T) {
	path := t.TempDir()
	dir, err := OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	child, err := dir.CreateDirectory("child", 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	file, err := child.CreateExclusive("file", 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := child.RemoveSelf(); !errors.Is(err, unix.ENOTEMPTY) {
		t.Fatalf("nonempty remove: %v", err)
	}
	if err := os.Rename(filepath.Join(path, "child"), filepath.Join(path, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(path, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		names, err := child.ReadNames()
		if err != nil || !slices.Equal(names, []string{"file"}) {
			t.Fatal(names, err)
		}
	}
	if err := child.Remove("file"); err != nil {
		t.Fatal(err)
	}
	if err := child.RemoveSelf(); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("replacement directory removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "child")); err != nil {
		t.Fatal(err)
	}
	empty, err := dir.CreateDirectory("empty", 0700)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	if err := empty.RemoveSelf(); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxReplaceAndExclusiveCreation(t *testing.T) {
	dir, err := OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	for name, data := range map[string]string{"source": "new", "target": "old"} {
		file, err := dir.CreateExclusive(name, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeAndSync(file, []byte(data)); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
	if file, err := dir.CreateExclusive("target", 0600); !errors.Is(err, os.ErrExist) {
		if file != nil {
			file.Close()
		}
		t.Fatal(err)
	}
	if err := dir.Replace("source", "target"); err != nil {
		t.Fatal(err)
	}
	file, err := dir.Open("target")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(data) != "new" {
		t.Fatal(string(data), err)
	}
	if err := dir.Remove("target"); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxConcurrentNoReplaceHasOneWinner(t *testing.T) {
	dir, err := OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	const writers = 8
	var workers sync.WaitGroup
	results := make(chan error, writers)
	for i := 0; i < writers; i++ {
		file, name, err := dir.CreateTemporary(".race-", 0600)
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
		workers.Add(1)
		go func() { defer workers.Done(); results <- dir.PromoteNoReplace(name, "winner") }()
	}
	workers.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("winners: %d", winners)
	}
}

func TestLinuxCloseWaitsOnlyForRootOperation(t *testing.T) {
	dir, err := OpenDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	file, err := dir.CreateExclusive("file", 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	entered, release, metadata, releaseMetadata := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	opened := make(chan *os.File, 1)
	go func() {
		file, err := dir.OpenObserved("file", func(stage string) {
			if stage == "native" {
				close(entered)
				<-release
			}
			if stage == "info" {
				close(metadata)
				<-releaseMetadata
			}
		})
		if err != nil {
			t.Error(err)
		}
		opened <- file
	}()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- dir.Close() }()
	select {
	case err := <-closed:
		t.Error("Close interrupted native open", err)
		closed <- err
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-metadata:
	case file := <-opened:
		if file != nil {
			file.Close()
		}
		close(releaseMetadata)
		t.Fatal("child open failed before metadata validation")
	case <-time.After(2 * time.Second):
		close(releaseMetadata)
		t.Fatal("child open did not reach metadata validation")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Close waited for independent child metadata")
	}
	close(releaseMetadata)
	file = <-opened
	if file == nil {
		t.Fatal("child open failed")
	}
	defer file.Close()
	if _, err := file.Stat(); err != nil {
		t.Fatal(err)
	}
	if dir.Handle() != 0 {
		t.Error("closed directory exposed fd")
	}
	if _, err := dir.OpenObserved("file", func(string) { t.Error("closed root reached native open") }); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	if _, err := dir.ReadNames(); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	if err := dir.RemoveSelf(); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	if err := dir.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxClosedDirectoryRejectsAllOperations(t *testing.T) {
	for _, dir := range []*Directory{nil, {}} {
		checks := []func() error{
			func() error { _, err := dir.Open("file"); return err },
			func() error { _, err := dir.OpenReadWrite("file"); return err },
			func() error { _, err := dir.OpenLock("file", true); return err },
			func() error { _, err := dir.CreateLock("file", 0600); return err },
			func() error { _, err := dir.CreateExclusive("file", 0600); return err },
			func() error { _, _, err := dir.CreateTemporary("tmp", 0600); return err },
			func() error { _, err := dir.CreateDirectory("child", 0700); return err },
			func() error { return dir.Replace("file", "target") },
			func() error { return dir.PromoteNoReplace("file", "target") },
			func() error { return dir.Remove("file") },
			func() error { return dir.RemoveSelf() },
			func() error { _, err := dir.ReadNames(); return err },
		}
		for i, check := range checks {
			if err := check(); !errors.Is(err, os.ErrClosed) {
				t.Errorf("operation %d: %v", i, err)
			}
		}
		if dir.Handle() != 0 {
			t.Error("closed directory exposed fd")
		}
		if err := dir.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
