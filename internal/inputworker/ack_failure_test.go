package inputworker

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"testing"
)

func TestUnconfirmedACKPreservesTransportFailureWithoutGuessingApplication(t *testing.T) {
	for _, tc := range []struct {
		cause error
		code  string
	}{
		{os.ErrDeadlineExceeded, "installed_input_failure_ack_timeout"},
		{io.EOF, "installed_input_failure_ack_eof"},
		{io.ErrUnexpectedEOF, "installed_input_failure_ack_eof"},
		{net.ErrClosed, "installed_input_failure_ack_closed"},
		{ErrProtocol, "installed_input_failure_ack_unclassified"},
		{nil, "installed_input_failure_ack_mismatch"},
	} {
		cause := tc.cause
		if cause != nil {
			cause = errors.Join(ErrProtocol, cause)
		}
		if got := ackFailureCode(cause); got != tc.code {
			t.Fatal("ack cause collapsed", got)
		}
	}
	if _, err := readResponse(bytes.NewReader([]byte{1})); !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, ErrProtocol) {
		t.Fatal("header read discarded transport evidence", err)
	}
	var wire bytes.Buffer
	_ = writeResponse(&wire, response{op: opCursorRead, sequence: 1, x: 1, y: 2})
	if _, err := readResponse(bytes.NewReader(wire.Bytes()[:responseSize+1])); !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, ErrProtocol) {
		t.Fatal("suffix read discarded transport evidence", err)
	}
}
