package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"time"
)

// Only the native pipe's pre-submission deadline checks may create this proof.
// A zero-byte result from an attempted WriteFile is not equivalent.
type unsubmittedPipeWrite struct{ cause error }

func (e *unsubmittedPipeWrite) Error() string { return "pipe write not submitted" }
func (e *unsubmittedPipeWrite) Unwrap() error { return e.cause }

func pipeAdmissionFailure(read bool, cause error) error {
	if !read && errors.Is(cause, os.ErrDeadlineExceeded) {
		return &unsubmittedPipeWrite{cause: cause}
	}
	return cause
}

// An attempted write remains fatal: zero returned bytes cannot prove the worker
// never received a command. Keep its cause for diagnostics, never for replay.
type sourceIOFailure struct {
	stage       string
	cause       error
	unsubmitted bool
	partial     bool
}

func (e *sourceIOFailure) Error() string { return "source command write failed" }
func (e *sourceIOFailure) Unwrap() error { return e.cause }

func writeSourceCommand(stream net.Conn, data []byte, deadline time.Time) error {
	stage := "deadline"
	unsubmitted := false
	partial := false
	err := stream.SetWriteDeadline(deadline)
	if err == nil {
		stage = "write"
		var n int
		n, err = stream.Write(data)
		partial = n > 0 && n < len(data)
		if n == 0 && err != nil {
			var proof *unsubmittedPipeWrite
			unsubmitted = errors.As(err, &proof)
		}
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		return nil
	}
	return &sourceIOFailure{stage: stage, cause: errors.Join(ErrAuthority, err), unsubmitted: unsubmitted, partial: partial}
}

// Closed labels only. Partial/attempted writes remain fatal; these observations
// cannot authorize replay or alter independent native-retirement evidence.
func (e *sourceIOFailure) operation() string {
	switch {
	case e.stage == "deadline":
		return "source-write-deadline"
	case e.unsubmitted:
		return "source-write-unsubmitted"
	case e.partial:
		return "source-write-partial"
	}
	switch sourceIOFailureReason(e.cause) {
	case "timeout":
		return "source-write-timeout"
	case "closed":
		return "source-write-closed"
	case "native":
		return "source-write-native"
	default:
		return "source-write-other"
	}
}

func sourceIOFailureReason(err error) string {
	var timeout net.Error
	var native syscall.Errno
	switch {
	case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded), errors.As(err, &timeout) && timeout.Timeout():
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, net.ErrClosed), errors.Is(err, io.ErrClosedPipe), errors.Is(err, io.EOF):
		return "closed"
	case errors.Is(err, io.ErrShortWrite):
		return "short_write"
	case errors.As(err, &native):
		return "native"
	default:
		return "other"
	}
}
