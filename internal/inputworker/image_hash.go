package inputworker

import (
	"crypto/sha256"
	"crypto/subtle"
	"io"
)

const imageHashReadSize = 256 << 10

// Hash every byte through the caller's live write/delete-excluding pin. The
// extra byte still detects growth; no pathname/digest cache replaces hashing.
// Larger reads amortize Windows file I/O without allocating the whole image.
func verifyImageHash(reader io.Reader, size int64, expected [sha256.Size]byte) error {
	if reader == nil || size <= 0 || size > 64<<20 {
		return ErrAuthority
	}
	hash := sha256.New()
	n, err := io.CopyBuffer(hash, io.LimitReader(reader, size+1), make([]byte, imageHashReadSize))
	if err != nil || n != size || subtle.ConstantTimeCompare(hash.Sum(nil), expected[:]) != 1 {
		return ErrAuthority
	}
	return nil
}
