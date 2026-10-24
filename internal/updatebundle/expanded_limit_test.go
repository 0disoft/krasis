package updatebundle

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestExpandedLimitRejectsArchiveBeforeEntryReads(t *testing.T) {
	var raw bytes.Buffer
	w := zip.NewWriter(&raw)
	for _, name := range installerNames {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		if strings.HasSuffix(name, ".exe") {
			header.UncompressedSize64 = uint64(maxExecutableBytes)
		}
		if _, err := w.CreateRaw(header); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, disk := range []bool{false, true} {
		t.Run(map[bool]string{false: "memory", true: "disk"}[disk], func(t *testing.T) {
			source := &entryHeaderObservedArchive{Reader: bytes.NewReader(raw.Bytes())}
			var err error
			if disk {
				parent, openErr := os.OpenRoot(t.TempDir())
				if openErr != nil {
					t.Fatal(openErr)
				}
				defer parent.Close()
				_, err = readStaged(source, int64(raw.Len()), parent, strings.Repeat("a", 32), func(string, io.ReaderAt, string) error { return nil })
			} else {
				_, err = read(source, int64(raw.Len()), func(string, []byte, string) error { return nil })
			}
			if !errors.Is(err, ErrInvalid) || source.entryReads != 0 {
				t.Fatalf("oversized archive: error=%v, entry header reads=%d, want 0", err, source.entryReads)
			}
		})
	}
}

type entryHeaderObservedArchive struct {
	*bytes.Reader
	entryReads int
}

func TestExpandedLimitMetadataBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		sizes []uint64
		valid bool
	}{
		{"exact_limit", []uint64{60 << 20, 60 << 20, 60 << 20, 12 << 20}, true},
		{"over_limit", []uint64{60 << 20, 60 << 20, 60 << 20, (12 << 20) + 1}, false},
		{"uint64_overflow", []uint64{1, ^uint64(0)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var entries []*zip.File
			for _, size := range tc.sizes {
				entries = append(entries, &zip.File{FileHeader: zip.FileHeader{UncompressedSize64: size}})
			}
			if got := archiveWithinExpandedLimit(entries); got != tc.valid {
				t.Fatalf("expanded sizes %v: accepted=%t, want %t", tc.sizes, got, tc.valid)
			}
		})
	}
}

func (r *entryHeaderObservedArchive) ReadAt(p []byte, offset int64) (int, error) {
	// ZIP entry.Open reads the fixed 30-byte local header before any contents.
	if len(p) == 30 {
		r.entryReads++
	}
	return r.Reader.ReadAt(p, offset)
}

func BenchmarkRejectExpandedArchive240MiB(b *testing.B) {
	var raw bytes.Buffer
	w := zip.NewWriter(&raw)
	w.RegisterCompressor(zip.Deflate, func(destination io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(destination, flate.BestSpeed)
	})
	for _, name := range installerNames {
		entry, err := w.Create(name)
		if err != nil {
			b.Fatal(err)
		}
		if strings.HasSuffix(name, ".exe") {
			if _, err := io.CopyN(entry, expandedZeroReader{}, maxExecutableBytes); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Read(bytes.NewReader(raw.Bytes()), int64(raw.Len())); !errors.Is(err, ErrInvalid) {
			b.Fatal("oversized archive was not rejected", err)
		}
	}
}

type expandedZeroReader struct{}

func (expandedZeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}
