package productlog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/productidentity"
)

func TestRequestPruneResumesAfterPartialPairRemoval(t *testing.T) {
	base := t.TempDir()
	id, peer := strings.Repeat("a", 32), strings.Repeat("b", 64)
	local := requestRetentionFixture(t, base, "support-request-"+id+"-local.json", 0, true)
	remote := requestRetentionFixture(t, base, "support-request-"+id+"-peer-"+peer+".json", 0, true)
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	if err = markRequestPrune(dir, id); err != nil {
		t.Fatal(err)
	}
	names, err := dir.ReadNames()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	err = finishRequestPrune(dir, id, names, func(name string) error {
		calls++
		if calls == 2 {
			return errors.New("injected sharing violation")
		}
		return dir.Remove(name)
	})
	if err == nil || calls != 2 {
		t.Fatal("failure not exercised", err, calls)
	}
	dir.Close()
	paths, _ := productidentity.ResolveWindowsPaths(base)
	marker := filepath.Join(paths.Diagnostics, requestPruneName(id))
	if _, err = os.Stat(marker); err != nil {
		t.Fatal("unfinished pruning checkpoint lost", err)
	}
	// New collection opens a fresh directory and completes the recorded pair.
	if _, err = ExportForRequest(base, strings.Repeat("c", 32)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{local, remote, marker} {
		if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("partial pruning not resumed", path, err)
		}
	}
}

func TestRequestPruneResumesMultipleGroupsAndEmptyCheckpoint(t *testing.T) {
	base := t.TempDir()
	peer := strings.Repeat("b", 64)
	keep := requestRetentionFixture(t, base, "support-request-"+strings.Repeat("f", 32)+"-local.json", 0, true)
	var removed []string
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("%032x", i)
		if i < 2 {
			for _, suffix := range []string{"-local.json", "-peer-" + peer + ".json"} {
				removed = append(removed, requestRetentionFixture(t, base, "support-request-"+id+suffix, 0, true))
			}
		}
		dir, err := productidentity.PrepareDiagnostics(base)
		if err != nil {
			t.Fatal(err)
		}
		err = markRequestPrune(dir, id)
		closeErr := dir.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		paths, _ := productidentity.ResolveWindowsPaths(base)
		removed = append(removed, filepath.Join(paths.Diagnostics, requestPruneName(id)))
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err = resumeRequestPruning(dir); err != nil {
		t.Fatal(err)
	}
	for _, path := range removed {
		if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("unfinished group retained", path, err)
		}
	}
	if _, err = os.Stat(keep); err != nil {
		t.Fatal("unmarked request removed", err)
	}
}

func TestRequestPruneRejectsInvalidCheckpointBeforeRemovingFiles(t *testing.T) {
	base := t.TempDir()
	id := strings.Repeat("a", 32)
	path := requestRetentionFixture(t, base, "support-request-"+id+"-local.json", 0, true)
	marker := requestRetentionFixture(t, base, requestPruneName(id), 0, true) // Nonempty checkpoint.
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err = resumeRequestPruning(dir); err == nil {
		t.Fatal("invalid checkpoint accepted")
	}
	for _, name := range []string{path, marker} {
		if _, err = os.Stat(name); err != nil {
			t.Fatal("invalid checkpoint must retain data", name, err)
		}
	}
}
