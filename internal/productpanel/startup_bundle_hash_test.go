package productpanel

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"testing"
)

type growingBundleReader struct{ read int64 }

func (r *growingBundleReader) Read(data []byte) (int, error) {
	clear(data)
	r.read += int64(len(data))
	return len(data), nil // The stream keeps growing; only the verifier can bound it.
}

func TestStartupBundleHashBoundsActualReads(t *testing.T) {
	boundary := &growingBundleReader{}
	if digest, err := hashStartupBundleImage(io.LimitReader(boundary, maxStartupImageBytes)); err != nil || digest == ([sha256.Size]byte{}) || boundary.read != maxStartupImageBytes {
		t.Fatal("image at the exact limit was rejected", boundary.read, err)
	}
	reader := &growingBundleReader{}
	digest, err := hashStartupBundleImage(reader)
	if !errors.Is(err, errStartupBundleFile) || digest != ([sha256.Size]byte{}) {
		t.Fatal("oversized image returned a usable digest", digest, err)
	}
	if reader.read != maxStartupImageBytes+1 {
		t.Fatal("verifier exceeded its overflow-detection byte", reader.read)
	}
}

func TestStartupBundleHashPreservesDigestAndReadFailure(t *testing.T) {
	data := []byte("complete executable bytes")
	digest, err := hashStartupBundleImage(bytes.NewReader(data))
	if err != nil || digest != sha256.Sum256(data) {
		t.Fatal("complete image digest changed", digest, err)
	}
	want := errors.New("image read failed")
	digest, err = hashStartupBundleImage(io.MultiReader(bytes.NewReader(data), bundleReadFailure{want}))
	if !errors.Is(err, want) || digest != ([sha256.Size]byte{}) {
		t.Fatal("partial image returned a usable digest", digest, err)
	}
}

type bundleReadFailure struct{ err error }

func (r bundleReadFailure) Read([]byte) (int, error) { return 0, r.err }
