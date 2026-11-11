package wininput

import (
	"io"
	"net"
)

// Pure error classification stays portable so it is tested on every host.
// Only use after the X11 owner is closed: XGB's canceled Reply/Check reports
// io.EOF, whereas a later operation on that socket reports net.ErrClosed.
// Independent native/cleanup failures must never disappear in a joined error.
func linuxSourceCancellationOnly(err, canceled error) bool {
	if canceled == nil {
		return false
	}
	if err == nil || err == canceled || err == net.ErrClosed || err == io.EOF {
		return true
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			if !linuxSourceCancellationOnly(child, canceled) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return linuxSourceCancellationOnly(wrapped.Unwrap(), canceled)
	default:
		return false
	}
}
