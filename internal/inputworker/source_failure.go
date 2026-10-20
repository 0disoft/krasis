package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"os"

	"github.com/0disoft/krasis/internal/wininput"
)

// Read only by the reader owner after its terminal error is known. Fixed
// categories preserve the cause without exporting error text or input values.
func sourceClientReaderCode(err error, retired bool) string {
	return "installed_source_reader_" + sourceFailureReason(err, retired)
}

func sourceStartupFailureCode(err error) string {
	return "installed_source_startup_" + sourceFailureReason(err, false)
}

func sourceFailureReason(err error, retired bool) string {
	reason := "other"
	switch {
	case errors.Is(err, wininput.ErrEmergencyRelease):
		reason = "emergency"
	case sourceCleanupUncertain(err):
		reason = "cleanup"
	case retired && err == nil:
		reason = "retired"
	case errors.Is(err, io.ErrUnexpectedEOF):
		reason = "truncated"
	case errors.Is(err, io.EOF):
		reason = "eof"
	case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		reason = "timeout"
	case errors.Is(err, net.ErrClosed), errors.Is(err, os.ErrClosed):
		reason = "closed"
	case errors.Is(err, context.Canceled):
		reason = "cancelled"
	case errors.Is(err, ErrProtocol):
		reason = "protocol"
	case errors.Is(err, ErrAuthority):
		reason = "authority"
	}
	return reason
}

// Fixed protocol operations only. This never includes a key/button value,
// cursor coordinate, peer identity, native error text or caller-provided string.
func sourceCommandFailureCode(op byte) string {
	name := "unknown"
	switch op {
	case sourceStart:
		name = "start"
	case sourceAdmit:
		name = "admit"
	case sourceActivate:
		name = "activate"
	case sourceDeactivate:
		name = "deactivate"
	case sourceHeartbeat:
		name = "heartbeat"
	case sourceStop:
		name = "stop"
	case sourceCursorPrepare:
		name = "cursor_prepare"
	case sourceCursorShow:
		name = "cursor_show"
	case sourceCursorRestore:
		name = "cursor_restore"
	}
	return "installed_source_command_failed_" + name
}
