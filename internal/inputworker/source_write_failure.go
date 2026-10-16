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

// A failed write remains fatal: zero returned bytes cannot prove the worker
// never received a command. Keep its cause for diagnostics, never for replay.
type sourceIOFailure struct {
	stage string
	cause error
}

func (e *sourceIOFailure) Error() string { return "source command write failed" }
func (e *sourceIOFailure) Unwrap() error { return e.cause }

func writeSourceCommand(stream net.Conn, data []byte, deadline time.Time) error {
	stage := "deadline"
	err := stream.SetWriteDeadline(deadline)
	if err == nil {
		stage = "write"
		var n int
		n, err = stream.Write(data)
		if err == nil && n != len(data) {
			err = io.ErrShortWrite
		}
	}
	if err == nil {
		return nil
	}
	return &sourceIOFailure{stage: stage, cause: errors.Join(ErrAuthority, err)}
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
