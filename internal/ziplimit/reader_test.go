package ziplimit

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"
)

type observedReaderAt struct {
	io.ReaderAt
	bytes int
}

func (r *observedReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	n, err := r.ReaderAt.ReadAt(p, offset)
	r.bytes += n
	return n, err
}

func directoryFixture(t testing.TB, entries int, comment string) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	if err := w.SetComment(comment); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < entries; i++ {
		if _, err := w.CreateRaw(&zip.FileHeader{Name: fmt.Sprintf("file%d", i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestReaderAtPreservesBoundariesCommentsAndPrefixes(t *testing.T) {
	for _, count := range []int{0, 1, 8, 9} {
		for _, comment := range []string{"", strings.Repeat("comment", 9000)} {
			data := directoryFixture(t, count, comment)
			for _, prefix := range []string{"", "executable prefix"} {
				candidate := append([]byte(prefix), data...)
				reader, err := NewReader(bytes.NewReader(candidate), int64(len(candidate)), 8)
				want := count > 0 && count <= 8
				if (err == nil) != want || WithinEntryLimit(candidate, 8) != want {
					t.Fatalf("count=%d comment=%d prefix=%d: %v", count, len(comment), len(prefix), err)
				}
				if want && len(reader.File) != count {
					t.Fatal("wrong entry count")
				}
			}
		}
	}
}

func TestReaderAtBoundsWrappedDirectoryMetadata(t *testing.T) {
	data := directoryFixture(t, 65537, "")
	end := len(data) - 22
	binary.LittleEndian.PutUint16(data[end+8:], 1)
	binary.LittleEndian.PutUint16(data[end+10:], 1)
	baseline, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(baseline.File) != 65537 {
		t.Fatalf("wrapped-count fixture: %v", err)
	}
	for _, limit := range []int{8, 16, 512} {
		source := &observedReaderAt{ReaderAt: bytes.NewReader(data)}
		if reader, err := NewReader(source, int64(len(data)), limit); err == nil || reader != nil {
			t.Fatal("wrapped count bypassed limit", limit)
		}
		// Footer windows plus fixed-size ZIP64 records and two candidate starts.
		budget := 66*1024 + 22 + 20 + 56 + 2*(limit+1)*46
		if source.bytes > budget {
			t.Fatalf("limit=%d read=%d, metadata budget=%d", limit, source.bytes, budget)
		}
	}
}

func TestReaderAtRejectsTruncationAndInvalidArguments(t *testing.T) {
	data := directoryFixture(t, 1, "")
	for _, size := range []int64{-1, 0, 21, int64(len(data)) + 1} {
		if _, err := NewReader(bytes.NewReader(data), size, 8); err == nil {
			t.Fatal("invalid size accepted", size)
		}
	}
	if _, err := NewReader(nil, int64(len(data)), 8); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := NewReader(bytes.NewReader(data), int64(len(data)), 0); err == nil {
		t.Fatal("zero entry limit accepted")
	}
}

func BenchmarkUpdateDirectoryRejection(b *testing.B) {
	data := directoryFixture(b, 65537, "")
	end := len(data) - 22
	binary.LittleEndian.PutUint16(data[end+8:], 1)
	binary.LittleEndian.PutUint16(data[end+10:], 1)
	for _, bounded := range []bool{false, true} {
		b.Run(fmt.Sprintf("bounded=%t", bounded), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				source := bytes.NewReader(data)
				if bounded {
					if _, err := NewReader(source, int64(len(data)), 16); err == nil {
						b.Fatal("oversized directory accepted")
					}
				} else if reader, err := zip.NewReader(source, int64(len(data))); err != nil || len(reader.File) != 65537 {
					b.Fatal("invalid baseline", err)
				}
			}
		})
	}
}

func FuzzReaderAtEntryLimit(f *testing.F) {
	f.Add([]byte(nil))
	f.Add(directoryFixture(f, 1, ""))
	f.Add(directoryFixture(f, 8, "comment"))
	f.Add(directoryFixture(f, 9, ""))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		allowed := WithinEntryLimit(data, 8)
		reader, err := NewReader(bytes.NewReader(data), int64(len(data)), 8)
		if err == nil && (!allowed || len(reader.File) > 8) {
			t.Fatal("reader bypassed preflight")
		}
		if allowed {
			baseline, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err == nil && len(baseline.File) <= 8 && reader == nil {
				t.Fatal("ReaderAt and byte preflight disagreed")
			}
		}
	})
}
