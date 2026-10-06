package productlog

import (
	"errors"
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
	calls := 0
	err = finishRequestPrune(dir, id, func(name string) error {
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
