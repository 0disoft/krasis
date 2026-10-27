package productpanel

import (
	"crypto/sha256"
	"errors"
	"io"
)

const maxStartupImageBytes int64 = 64 << 20

var errStartupBundleFile = errors.New("invalid bundle file")

// Enforce the limit on bytes read, even if an opened image grows after Stat.
// Hashing retains only io.Copy's fixed buffer, never the complete executable.
func hashStartupBundleImage(reader io.Reader) ([sha256.Size]byte, error) {
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(reader, maxStartupImageBytes+1))
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	if n > maxStartupImageBytes {
		return [sha256.Size]byte{}, errStartupBundleFile
	}
	var digest [sha256.Size]byte
	hash.Sum(digest[:0])
	return digest, nil
}
