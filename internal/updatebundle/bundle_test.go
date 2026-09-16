package updatebundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, mutate func(map[string][]byte), extra string) []byte {
	t.Helper()
	files := map[string][]byte{}
	for _, name := range names {
		files[name] = []byte("test fixture")
	}
	files["BUILD.json"], _ = json.Marshal(Metadata{Revision: strings.Repeat("a", 40), Platform: "windows/amd64", Purpose: "manual-two-pc-smoke", Generation: 21})
	var sums strings.Builder
	for _, name := range names[:len(names)-1] {
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(files[name]), name)
	}
	files["SHA256SUMS.txt"] = []byte(sums.String())
	if mutate != nil {
		mutate(files)
	}
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for _, name := range names {
		if data, ok := files[name]; ok {
			f, err := w.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			f.Write(data)
		}
	}
	if extra != "" {
		f, _ := w.Create(extra)
		f.Write([]byte("extra"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestBundleRejectsUnsafeAndCorruptArchives(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		mutate      func(map[string][]byte)
	}{
		{name: "checksum", mutate: func(f map[string][]byte) { f["krasis.exe"] = []byte("tampered") }},
		{name: "missing", mutate: func(f map[string][]byte) { delete(f, "krasis.exe") }},
		{name: "traversal", extra: "../krasis.exe"},
		{name: "duplicate", extra: "krasis.exe"},
		{name: "case alias", extra: "KRASIS.EXE"},
		{name: "ADS", extra: "krasis.exe:other"},
		{name: "unknown", extra: "run.ps1"},
		{name: "large document", mutate: func(f map[string][]byte) { f["BUILD.json"] = bytes.Repeat([]byte("x"), 8193) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := fixture(t, tc.mutate, tc.extra)
			if _, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil }); err == nil {
				t.Fatal("accepted invalid bundle")
			}
		})
	}
}

func TestBundleStagesOnlyVerifiedFixedNamesWithoutOverwrite(t *testing.T) {
	data := fixture(t, nil, "")
	b, err := read(bytes.NewReader(data), int64(len(data)), func(string, []byte, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	id := strings.Repeat("b", 32)
	if err := b.Stage(root, id); err != nil {
		t.Fatal(err)
	}
	if err := b.Stage(root, id); err == nil {
		t.Fatal("overwrote previous version")
	}
	if err := b.Stage(root, "../escape"); err == nil {
		t.Fatal("accepted arbitrary destination")
	}
	f, err := root.Open(id + "/krasis.exe")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := Read(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("production validator accepted fake executable")
	}
	if _, err := Read(bytes.NewReader(data), MaxArchiveBytes+1); err == nil {
		t.Fatal("accepted oversized archive")
	}
}
