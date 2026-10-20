//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestSourceWriteReportPreservesFailureBeforeRetirement(t *testing.T) {
	for _, tc := range []struct {
		name              string
		cause             error
		deadline, partial bool
		operation         string
	}{
		{"configure", syscall.Errno(6), true, false, "source-write-deadline"},
		{"unsubmitted", &unsubmittedPipeWrite{cause: os.ErrDeadlineExceeded}, false, false, "source-write-unsubmitted"},
		{"attempted timeout", os.ErrDeadlineExceeded, false, false, "source-write-timeout"},
		{"partial", syscall.Errno(109), false, true, "source-write-partial"},
		{"closed", io.ErrClosedPipe, false, false, "source-write-closed"},
		{"native", syscall.Errno(6), false, false, "source-write-native"},
		{"private", errors.New("private-path-peer-input"), false, false, "source-write-other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			defer conn.Close()
			stream := &failingSourceStream{Conn: conn, cause: tc.cause, deadline: tc.deadline, partial: tc.partial}
			failure := writeSourceCommand(stream, make([]byte, sourcePacketSize), time.Now().Add(time.Second))
			loop := sourceCommandFailed(sourceHeartbeat, sourceTransportFailure(failure))
			final := sourceCaptureFinal(loop, context.Canceled, context.Canceled, false)
			final = sourceRetirementDelivery(final, &sourceIOFailure{stage: "write", cause: syscall.Errno(109)})
			if sourceRetirementStatus(final) != 0 || errors.Is(final, ErrRecovery) {
				t.Fatal("joined transport failure changed cleanup evidence", final)
			}
			var wire bytes.Buffer
			if err := writeReport(&wire, Report{}, final); err != nil {
				t.Fatal(err)
			}
			_, received := readReport(&wire)
			var detail interface{ LoginRuntimeOperation() string }
			if !errors.As(received, &detail) || detail.LoginRuntimeOperation() != tc.operation || !logindiag.ValidRuntimeOperation(tc.operation) {
				t.Fatal("original write stage lost to retirement", received, tc.operation)
			}
			if strings.Contains(string(wire.Bytes()), "private") {
				t.Fatal("private error entered report")
			}
			wantWrites := 1
			if tc.deadline {
				wantWrites = 0
			}
			if stream.writes != wantWrites {
				t.Fatal("write replayed", stream.writes)
			}
			// A separate uncertain native join must keep its stronger cause.
			unsafe := sourceCaptureFinal(loop, inputdesktop.ErrCleanup, context.Canceled, false)
			op, _ := reportFailureDetails(unsafe)
			if strings.HasPrefix(reportOperations[op], "source-write-") || !errors.Is(unsafe, ErrRecovery) {
				t.Fatal("transport detail hid native cleanup uncertainty", unsafe)
			}
		})
	}
}
