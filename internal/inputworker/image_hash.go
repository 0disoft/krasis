package inputworker

import (
	"crypto/sha256"
	"crypto/subtle"
	"io"
	"time"
)

const imageHashReadSize = 256 << 10

// Hash every byte through the caller's live write/delete-excluding pin. The
// extra byte still detects growth; no pathname/digest cache replaces hashing.
// Larger reads amortize Windows file I/O without allocating the whole image.
func verifyImageHash(reader io.Reader, size int64, expected [sha256.Size]byte) error {
	return verifyImageHashMeasured(reader, size, expected, nil)
}

type imageHashTimings struct {
	read, compute time.Duration
	backend       uint8
}

type imageDigest struct {
	write   func([]byte) error
	finish  func() ([sha256.Size]byte, error)
	close   func() error
	backend uint8
}

func newGoImageDigest() (*imageDigest, error) {
	h := sha256.New()
	return &imageDigest{backend: 1, write: func(b []byte) error { _, err := h.Write(b); return err },
		finish: func() ([sha256.Size]byte, error) {
			var sum [sha256.Size]byte
			copy(sum[:], h.Sum(nil))
			return sum, nil
		},
		close: func() error { return nil }}, nil
}

func verifyImageHashMeasured(reader io.Reader, size int64, expected [sha256.Size]byte, timing *imageHashTimings) (result error) {
	return verifyImageHashUsing(reader, size, expected, timing, newImageDigest)
}

func verifyImageHashUsing(reader io.Reader, size int64, expected [sha256.Size]byte, timing *imageHashTimings, create func() (*imageDigest, error)) (result error) {
	if reader == nil || size <= 0 || size > 64<<20 {
		return ErrAuthority
	}
	var measured imageHashTimings
	started := time.Now()
	digest, err := create()
	measured.compute = time.Since(started)
	if err != nil {
		return ErrAuthority
	}
	measured.backend = digest.backend
	defer func() {
		started := time.Now()
		if digest.close() != nil {
			result = ErrAuthority
		}
		measured.compute += time.Since(started)
		if timing != nil {
			*timing = measured
		}
	}()
	r := &imageHashReader{reader: io.LimitReader(reader, size+1), timing: &measured}
	w := &imageHashWriter{digest: digest, timing: &measured}
	n, err := io.CopyBuffer(w, r, make([]byte, imageHashReadSize))
	if err != nil || n != size {
		return ErrAuthority
	}
	started = time.Now()
	sum, err := digest.finish()
	measured.compute += time.Since(started)
	if err != nil || subtle.ConstantTimeCompare(sum[:], expected[:]) != 1 {
		return ErrAuthority
	}
	return nil
}

type imageHashReader struct {
	reader io.Reader
	timing *imageHashTimings
}

func (r *imageHashReader) Read(b []byte) (int, error) {
	started := time.Now()
	n, err := r.reader.Read(b)
	r.timing.read += time.Since(started)
	return n, err
}

type imageHashWriter struct {
	digest *imageDigest
	timing *imageHashTimings
}

func (w *imageHashWriter) Write(b []byte) (int, error) {
	started := time.Now()
	err := w.digest.write(b)
	w.timing.compute += time.Since(started)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}
