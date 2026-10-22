package productlog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

func requestRetentionFixture(t *testing.T, base, name string, size int64, old bool) string {
	t.Helper()
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	f, err := dir.CreateExclusive(name, 0600)
	if err != nil {
		dir.Close()
		t.Fatal(err)
	}
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		t.Fatal(err)
	}
	if size > 0 {
		err = f.Truncate(size)
	} else {
		_, err = f.Write([]byte(`{"schema":"krasis.support/v1","created":"2026-10-07T00:00:00Z","logs":[]}`))
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = dir.Close(); err != nil {
		t.Fatal(err)
	}
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(paths.Diagnostics, name)
	if old {
		stamp := time.Now().Add(-time.Hour)
		if err = os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestRequestRetentionPrunesOldPairsAndKeepsActive(t *testing.T) {
	base := t.TempDir()
	peer := strings.Repeat("c", 64)
	var oldestLocal, oldestPeer string
	for i := 0; i < maxSupportRequests; i++ {
		id := fmt.Sprintf("%032x", i)
		local := requestRetentionFixture(t, base, "support-request-"+id+"-local.json", 0, i == 0)
		remote := requestRetentionFixture(t, base, "support-request-"+id+"-peer-"+peer+".json", 0, i == 0)
		if i == 0 {
			oldestLocal, oldestPeer = local, remote
		}
	}
	other := requestRetentionFixture(t, base, "support-latest.json", 0, true)
	if _, err := ExportForRequest(base, strings.Repeat("a", 32)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{oldestLocal, oldestPeer} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("old pair retained", err)
		}
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("unrelated snapshot pruned", err)
	}
	if _, err := ExportForRequest(base, strings.Repeat("b", 32)); !errors.Is(err, ErrRequestStorageLimit) {
		t.Fatal("active requests should fill count budget", err)
	}
}

func TestRequestRetentionReservesByteBudget(t *testing.T) {
	for _, old := range []bool{false, true} {
		base := t.TempDir()
		path := requestRetentionFixture(t, base, "support-request-"+strings.Repeat("c", 32)+"-local.json", maxRequestSupportBytes, old)
		_, err := ExportForRequest(base, strings.Repeat("a", 32))
		if old {
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("old bytes not pruned", err)
			}
		} else if !errors.Is(err, ErrRequestStorageLimit) {
			t.Fatal("active bytes pruned or budget exceeded", err)
		}
	}
}

func TestRequestRetentionPrunesMultipleGroupsInStableOrder(t *testing.T) {
	base := t.TempDir()
	stamp := time.Now().Add(-time.Hour)
	var paths []string
	for i := 0; i < maxSupportRequests+3; i++ {
		id := fmt.Sprintf("%032x", i)
		path := requestRetentionFixture(t, base, "support-request-"+id+"-local.json", 0, true)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	// An old local file with a recent peer file is still an active group.
	requestRetentionFixture(t, base, "support-request-"+fmt.Sprintf("%032x", 1)+"-peer-"+strings.Repeat("c", 64)+".json", 0, false)
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	// The current request is protected even though its timestamp is old.
	id := fmt.Sprintf("%032x", 0)
	if err = reserveRequestSnapshot(dir, id, filepath.Base(paths[0]), 0); err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		_, err := os.Stat(path)
		if i >= 2 && i <= 4 {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatal("oldest tied group retained", i, err)
			}
		} else if err != nil {
			t.Fatal("protected or newer group removed", i, err)
		}
	}
}
