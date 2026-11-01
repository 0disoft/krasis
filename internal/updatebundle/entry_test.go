package updatebundle

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestReadEntryEnforcesSizeAndPreservesReadErrors(t *testing.T) {
	failure := errors.New("entry read failed")
	for _, tc := range []struct {
		name string
		size uint64
		body string
		err  error
		want error
	}{
		{"empty", 0, "", io.EOF, nil},
		{"exact", 3, "abc", io.EOF, nil},
		{"short", 3, "ab", io.EOF, ErrInvalid},
		{"long", 3, "abcd", io.EOF, ErrInvalid},
		{"error_with_last_bytes", 3, "abc", failure, failure},
		{"checksum_failure", 3, "abc", zip.ErrChecksum, zip.ErrChecksum},
		{"over_limit", uint64(maxExecutableBytes) + 1, "", io.EOF, ErrInvalid},
		{"size_overflow", ^uint64(0), "", io.EOF, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &entryTerminalReader{data: []byte(tc.body), err: tc.err}
			data, err := readEntry(reader, tc.size)
			if tc.want == nil {
				if err != nil || string(data) != tc.body {
					t.Fatal("valid entry changed", data, err)
				}
			} else if !errors.Is(err, tc.want) || data != nil {
				t.Fatal("invalid entry or cause accepted", data, err)
			}
			if tc.size > uint64(maxExecutableBytes) && reader.reads != 0 {
				t.Fatal("oversized entry was read before rejection")
			}
		})
	}
}

type entryTerminalReader struct {
	data  []byte
	err   error
	reads int
}

func (r *entryTerminalReader) Read(p []byte) (int, error) {
	r.reads++
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

func TestReadEntryMatchesZIPCRCAndTruncationChecks(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: "sample", Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte("fixture content"))
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		data := archive.Bytes()
		if corrupt {
			index := bytes.Index(data, []byte("fixture content"))
			if index < 0 {
				t.Fatal("fixture has no stored body")
			}
			data[index] ^= 1
		}
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		stream, err := reader.File[0].Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := readEntry(stream, reader.File[0].UncompressedSize64)
		_ = stream.Close()
		if corrupt {
			if !errors.Is(err, zip.ErrChecksum) || body != nil {
				t.Fatal("ZIP CRC failure was hidden", err)
			}
		} else if err != nil || string(body) != "fixture content" {
			t.Fatal("valid ZIP entry changed", body, err)
		}
	}
}

func BenchmarkBundleEntryAllocation(b *testing.B) {
	for _, size := range []int64{1 << 20, 16 << 20} {
		for _, exact := range []bool{false, true} {
			b.Run(fmt.Sprintf("MiB=%d/exact=%t", size>>20, exact), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(size)
				for b.Loop() {
					stream := io.LimitReader(expandedZeroReader{}, size)
					var data []byte
					var err error
					if exact {
						data, err = readEntry(stream, uint64(size))
					} else {
						data, err = io.ReadAll(io.LimitReader(stream, maxExecutableBytes+1))
					}
					if err != nil || int64(len(data)) != size {
						b.Fatal("entry read failed", err)
					}
				}
			})
		}
	}
}
