package inputworker

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/0disoft/krasis/internal/crashguard"
)

const (
	preparationBootstrapFailure byte = 1
	preparationNativeFailure    byte = 2
)

var preparationFailureMagic = [4]byte{'K', 'R', 'F', '1'}

// Only the authenticated, still-unbound worker may send this negative report.
// It carries fixed enums and a Win32 result, never stderr, input or authority.
// It cannot substitute for the successful native-retirement receipt.
func reportPreparationFailure(control net.Conn, phase byte, cause error) error {
	frame := preparationFailureFrame(phase, cause)
	// Diagnostics must not consume the independent native-cleanup budget.
	if err := control.SetWriteDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
		return errors.Join(cause, err)
	}
	return errors.Join(cause, writeFrame(control, frame[:]))
}

func preparationFailureFrame(phase byte, cause error) (frame [12]byte) {
	copy(frame[:4], preparationFailureMagic[:])
	frame[4] = phase
	var pending *crashguard.ExitPending
	var code syscall.Errno
	switch {
	case errors.As(cause, &pending):
		frame[5] = 1
	case errors.Is(cause, context.DeadlineExceeded), errors.Is(cause, os.ErrDeadlineExceeded), errors.Is(cause, crashguard.ErrExitTimeout):
		frame[5] = 2
	case errors.Is(cause, context.Canceled):
		frame[5] = 3
	case errors.Is(cause, io.ErrUnexpectedEOF):
		frame[5] = 4
	case errors.Is(cause, io.EOF):
		frame[5] = 5
	case errors.Is(cause, net.ErrClosed), errors.Is(cause, os.ErrClosed):
		frame[5] = 6
	case errors.As(cause, &code) && code != 0 && uint64(code) <= 0xffffffff:
		frame[5] = 7
	}
	if errors.As(cause, &code) && uint64(code) <= 0xffffffff && code != 0 {
		frame[6] = 1
		binary.BigEndian.PutUint32(frame[8:], uint32(code))
	}
	return frame
}

func readPreparationRetirement(reader io.Reader, expected [8]byte) error {
	var header [8]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return errors.Join(ErrProtocol, err)
	}
	if header == expected {
		return nil
	}
	if [4]byte(header[:4]) != preparationFailureMagic || header[4] < 1 || header[4] > 2 || header[5] > 7 || header[6] > 1 || header[7] != 0 {
		return ErrProtocol
	}
	var data [4]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return errors.Join(ErrProtocol, err)
	}
	code := binary.BigEndian.Uint32(data[:])
	if (header[6] == 1) != (code != 0) || header[5] == 7 && code == 0 {
		return ErrProtocol
	}
	phase := "bootstrap"
	if header[4] == preparationNativeFailure {
		phase = "native"
	}
	kind := [...]string{"other", "pending", "timeout", "cancelled", "truncated", "eof", "closed", "win32"}[header[5]]
	var native error
	if code != 0 {
		native = syscall.Errno(code)
	}
	return inputServiceAt("input-retire-"+phase+"-"+kind, errors.Join(ErrRecovery, native))
}
