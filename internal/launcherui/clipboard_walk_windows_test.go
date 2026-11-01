//go:build windows

package launcherui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/0disoft/krasis/internal/peerupdate"
)

func TestClipboardWalkPreservesOrderAndExactBudget(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"z", "a", "a/z", "a/A"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	var want []string
	if err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		want = append(want, path)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, budget := range []int{0, len(want) - 1, len(want), len(want) + 1} {
		var got []string
		err := walkClipboardFiles(root, budget, func(path string, _ fs.DirEntry, err error) error {
			got = append(got, path)
			return err
		})
		if budget < len(want) {
			if !errors.Is(err, errClipboardFileCount) {
				t.Fatalf("budget %d: expected item limit, got %v", budget, err)
			}
		} else if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("budget %d changed traversal: %v %v", budget, got, err)
		}
	}
}

func TestClipboardWalkRejectsWideDirectoryBeforeVisitingChildren(t *testing.T) {
	root := clipboardWalkWideFixture(t, peerupdate.MaxClipboardFiles)
	visited := 0
	err := walkClipboardFiles(root, peerupdate.MaxClipboardFiles, func(_ string, _ fs.DirEntry, err error) error {
		visited++
		return err
	})
	if !errors.Is(err, errClipboardFileCount) || visited != 1 {
		t.Fatalf("wide tree was traversed before rejection: visits=%d err=%v", visited, err)
	}
	if data, err := packClipboardFiles([]string{root}); data != nil || !errors.Is(err, errClipboardFileCount) {
		t.Fatalf("oversized tree published or misclassified: bytes=%d err=%v", len(data), err)
	}
}

func TestClipboardWalkPreservesValidationFailures(t *testing.T) {
	root := t.TempDir()
	failure := errors.New("validation rejected directory")
	visited := 0
	err := walkClipboardFiles(root, 10, func(_ string, _ fs.DirEntry, err error) error {
		visited++
		return errors.Join(failure, err)
	})
	if !errors.Is(err, failure) || visited != 1 {
		t.Fatal("traversed past failed validation", visited, err)
	}
	err = walkClipboardFiles(filepath.Join(root, "missing"), 10, func(_ string, entry fs.DirEntry, err error) error {
		if entry != nil || !errors.Is(err, os.ErrNotExist) {
			t.Fatal("lost root lookup failure", entry, err)
		}
		return err
	})
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("lost root lookup cause", err)
	}
}

func TestClipboardWalkSharesPackingBudgetAcrossRoots(t *testing.T) {
	root := clipboardWalkWideFixture(t, peerupdate.MaxClipboardFiles-1)
	if data, err := packClipboardFiles([]string{root}); err != nil || !peerupdate.ValidClipboardFiles(data) {
		t.Fatal("rejected exactly 512 entries", err)
	}
	if data, err := packClipboardFiles([]string{root, t.TempDir()}); data != nil || !errors.Is(err, errClipboardFileCount) {
		t.Fatal("reset entry budget for the second selected root", err)
	}
}

func TestClipboardWalkKeepsDuplicateRootRejection(t *testing.T) {
	root := t.TempDir()
	if data, err := packClipboardFiles([]string{root, root}); data != nil || !errors.Is(err, errClipboardUnavailable) {
		t.Fatal("silently deduplicated selected roots", err)
	}
}

func clipboardWalkWideFixture(tb testing.TB, count int) string {
	tb.Helper()
	root := tb.TempDir()
	for i := 0; i < count; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("entry-%05d", count-i)), 0700); err != nil {
			tb.Fatal(err)
		}
	}
	return root
}

func BenchmarkClipboardWalkWideRejection(b *testing.B) {
	root := clipboardWalkWideFixture(b, 4096)
	for _, bounded := range []bool{false, true} {
		b.Run(fmt.Sprintf("bounded=%t", bounded), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				visited := 0
				visit := func(_ string, _ fs.DirEntry, err error) error {
					if visited >= peerupdate.MaxClipboardFiles {
						return errClipboardFileCount
					}
					visited++
					return err
				}
				var err error
				if bounded {
					err = walkClipboardFiles(root, peerupdate.MaxClipboardFiles, visit)
				} else {
					err = filepath.WalkDir(root, visit)
				}
				if !errors.Is(err, errClipboardFileCount) {
					b.Fatal(err)
				}
			}
		})
	}
}
