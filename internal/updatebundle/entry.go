package updatebundle

import (
	"errors"
	"io"
)

// The caller has checked the per-file and total expanded ZIP budgets. Allocate
// once for the declared size plus a sentinel, rather than grow/copy ReadAll's
// buffer repeatedly. Read through EOF so ZIP CRC failures are still observed.
func readEntry(stream io.Reader, size uint64) ([]byte, error) {
	if size > uint64(maxExecutableBytes) {
		return nil, ErrInvalid
	}
	data := make([]byte, int(size)+1)
	n := 0
	for n < len(data) {
		read, err := stream.Read(data[n:])
		n += read
		if err != nil {
			if err == io.EOF && n == int(size) {
				return data[:n], nil
			}
			return nil, errors.Join(ErrInvalid, err)
		}
	}
	return nil, ErrInvalid
}
