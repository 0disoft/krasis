package inputworker

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

type countedImageReader struct {
	reader  io.Reader
	calls   int
	largest int
}

func (r *countedImageReader) Read(data []byte) (int, error) {
	r.calls++
	r.largest = max(r.largest, len(data))
	return r.reader.Read(data)
}

func TestImageHashReadsWholeImageInBoundedChunks(t *testing.T) {
	data := bytes.Repeat([]byte{37}, 9<<20)
	reader := &countedImageReader{reader: bytes.NewReader(data)}
	if err := verifyImageHash(reader, int64(len(data)), sha256.Sum256(data)); err != nil {
		t.Fatal("matching full image rejected", err)
	}
	if reader.calls != 37 || reader.largest != 256<<10 {
		t.Fatal("expected 36 data reads and one growth/EOF check", reader.calls, reader.largest)
	}
}

func TestImageHashRejectsCorruptionTruncationGrowthAndReadErrors(t *testing.T) {
	data := []byte("protected image")
	digest := sha256.Sum256(data)
	for name, reader := range map[string]io.Reader{
		"changed":    bytes.NewReader([]byte("untrusted image")),
		"truncated":  bytes.NewReader(data[:len(data)-1]),
		"grown":      bytes.NewReader(append(append([]byte{}, data...), 1)),
		"read_error": io.MultiReader(bytes.NewReader(data), iotest.ErrReader(errors.New("read failure"))),
	} {
		t.Run(name, func(t *testing.T) {
			if verifyImageHash(reader, int64(len(data)), digest) != ErrAuthority {
				t.Fatal("invalid full-image proof accepted")
			}
		})
	}
	if verifyImageHash(iotest.OneByteReader(bytes.NewReader(data)), int64(len(data)), digest) != nil {
		t.Fatal("short successful reads changed digest verification")
	}
	for _, size := range []int64{0, -1, (64 << 20) + 1} {
		if verifyImageHash(bytes.NewReader(data), size, digest) != ErrAuthority {
			t.Fatal("invalid image size accepted", size)
		}
	}
}
