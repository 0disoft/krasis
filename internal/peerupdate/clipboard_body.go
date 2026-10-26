package peerupdate

import "io"

// The caller validates the advertised size against the format limit and owns
// the single receiving slot before allocating. Keep one extra byte to detect
// an oversized body, and preserve transport errors even at the expected size.
func readClipboardBody(r io.Reader, limit int) ([]byte, error) {
	data := make([]byte, limit+1)
	n := 0
	for n < len(data) {
		read, err := r.Read(data[n:])
		n += read
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return data[:n], err
		}
	}
	return data, nil
}
