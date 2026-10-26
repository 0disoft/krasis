package launchrecord

import "io"

const cliCopyBufferSize = 1 << 20

// The source stays open through atomic publication. Bound both memory and the
// exact byte count observed by Stat; a growing/truncated executable never
// replaces the existing recovery CLI. Fixed-size metadata keeps its small Write.
func copyCLIStream(output io.Writer, source io.Reader, size int64) error {
	if size <= 0 || size > maxExecutable {
		return ErrInvalid
	}
	// Hide os.File.ReadFrom so CopyBuffer actually uses our buffer, amortizing
	// the secure temporary file's synchronous write-through operations on Windows.
	buffer := make([]byte, min(size+1, cliCopyBufferSize))
	n, err := io.CopyBuffer(struct{ io.Writer }{output}, io.LimitReader(source, size+1), buffer)
	if err != nil {
		return err
	}
	if n != size {
		return ErrInvalid
	}
	return nil
}
