package runtimestall

import (
	"errors"
	"io"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

const FileName = "runtime-stall.trace"
const maxSnapshotBytes = 8 << 20

var errTraceLimit = errors.New("execution trace exceeds local snapshot limit")

type limitedWriter struct {
	dst       io.Writer
	remaining int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, errTraceLimit
	}
	n, err := w.dst.Write(p)
	w.remaining -= int64(n)
	if n != len(p) && err == nil {
		err = io.ErrShortWrite
	}
	return n, err
}

// Keep one bounded file across restarts. Publish only a complete snapshot;
// ordinary support JSON and peer diagnostics never enumerate trace files.
func save(base string, source traceSource) error {
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	f, name, err := dir.CreateTemporary(".runtime-trace-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(name)
	defer f.Close()
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		return err
	}
	if _, err = source.WriteTo(&limitedWriter{dst: f, remaining: maxSnapshotBytes}); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return dir.Replace(name, FileName)
}
