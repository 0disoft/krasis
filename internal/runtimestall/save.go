package runtimestall

import (
	"errors"
	"io"
	"runtime"
	"syscall"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

const FileName = "runtime-stall.trace"
const FallbackFileName = "runtime-stall-next.trace"
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

// Keep at most two bounded files across restarts. Publish only a complete snapshot;
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
	if err = dir.Replace(name, FileName); err == nil || !snapshotBlocked(err) {
		return err
	}
	// An offline reader may hold the previous snapshot without delete sharing.
	// Preserve that file and publish the already complete snapshot to one fixed
	// spare slot. Never broaden ACLs or retain an unbounded series of run files.
	return dir.Replace(name, FallbackFileName)
}

func snapshotBusy(err error) bool {
	return runtime.GOOS == "windows" && (errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33)))
}

func snapshotBlocked(err error) bool {
	// NT rename may report ACCESS_DENIED for an open destination too. It is
	// not proof of sharing contention. Only try our other fixed output slot;
	// never change the destination's permissions or remove its reader.
	return snapshotBusy(err) || runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(5))
}

func saveFailureCode(err error) string {
	switch {
	case errors.Is(err, errTraceLimit):
		return "runtime_trace_save_limit"
	case snapshotBusy(err):
		return "runtime_trace_save_busy"
	case runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(5)):
		return "runtime_trace_save_denied"
	default:
		return "runtime_trace_save_failed"
	}
}
