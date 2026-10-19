//go:build windows

package runtimestall

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/productlog"
)

func TestSnapshotReaderCannotDiscardNextCapture(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "Krasis", "diagnostics")
	if err := save(base, &fakeSource{payload: "old-runtime-record"}); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err = save(base, &fakeSource{payload: "new-runtime-record"}); err != nil {
		t.Fatal("reader blocked next capture", err)
	}
	for name, want := range map[string]string{FileName: "old-runtime-record", FallbackFileName: "new-runtime-record"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != want {
			t.Fatal("complete snapshot changed or lost", name, err)
		}
	}
	spare, err := os.Open(filepath.Join(dir, FallbackFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer spare.Close()
	if err = save(base, &fakeSource{payload: "third-runtime-record"}); !snapshotBlocked(err) {
		t.Fatal("both readers did not preserve busy failure", err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".runtime-trace-*"))
	if len(leftovers) != 0 {
		t.Fatal("failed publication left partial files", leftovers)
	}
	exported, err := productlog.Export(base)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exported)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{FileName, FallbackFileName, "runtime-record"} {
		if bytes.Contains(data, []byte(marker)) {
			t.Fatal("trace entered normal export", marker)
		}
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err = save(base, &fakeSource{payload: "latest-runtime-record"}); err != nil {
		t.Fatal("closed reader still blocks primary slot", err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.trace"))
	if len(files) != 2 {
		t.Fatal("trace slots grew without a bound", files)
	}
}
