//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"testing"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestSourceWriterFailureOriginSurvivesFinalReport(t *testing.T) {
	for _, origin := range []string{"boundary", "action", "reply", "return", "retirement"} {
		t.Run(origin, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			stream := &failingSourceStream{Conn: conn, cause: os.ErrDeadlineExceeded}
			writer := &sourceWriter{channel: clientChannel(stream, authorityAgent), binding: Binding{1}}
			defer writer.channel.Close()
			writer.started.Store(true)
			var failure error
			switch origin {
			case "boundary":
				failure = writer.sendBoundaries([]sourcePacket{boundaryBatchPacket(1)})
			case "action":
				failure = writer.send(sourcePacket{kind: sourceAction, reply: 1, action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}}, nil)
			case "reply":
				failure = writer.send(sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1}, nil)
			case "return":
				failure = writer.send(sourcePacket{kind: sourceReturn, reply: 1}, nil)
			case "retirement":
				failure = writer.write(sourcePacket{kind: sourceReply, op: sourceStop, reply: 1}, nil, true)
			}
			if !errors.Is(failure, os.ErrDeadlineExceeded) || stream.writes != 1 || !writer.broken {
				t.Fatal("diagnostics changed attempted-write failure policy", failure, stream.writes, writer.broken)
			}
			final := sourceCaptureFinal(sourceCommandFailed(sourceHeartbeat, failure), context.Canceled, context.Canceled, false)
			final = sourceRetirementDelivery(final, &sourceIOFailure{stage: "write", origin: sourceRetired, cause: net.ErrClosed})
			var wire bytes.Buffer
			if err := writeReport(&wire, Report{}, final); err != nil {
				t.Fatal(err)
			}
			_, received := readReport(&wire)
			code, _ := reportFailureDetails(received)
			want := "source-write-" + origin + "-timeout"
			if reportOperations[code] != want || !logindiag.ValidRuntimeOperation(want) {
				t.Fatal("first failed writer lost to retirement or report transport", reportOperations[code], want)
			}
			unsafe := sourceCaptureFinal(failure, inputdesktop.ErrCleanup, context.Canceled, false)
			code, _ = reportFailureDetails(unsafe)
			if !errors.Is(unsafe, ErrRecovery) || reportOperations[code] == want {
				t.Fatal("write origin hid native cleanup uncertainty", unsafe)
			}
		})
	}
}

func TestSourceWriteOriginAllowlistRejectsPrivateLabels(t *testing.T) {
	for _, operation := range logindiag.SourceWriteOriginOperations() {
		if !logindiag.ValidRuntimeOperation(operation) {
			t.Fatal("allowlisted origin cannot cross sanitized export", operation)
		}
	}
	for _, operation := range []string{"source-write-key-timeout", "source-write-action-private", "source-write-action-timeout-private", "source-write-private-path-timeout"} {
		if logindiag.ValidRuntimeOperation(operation) {
			t.Fatal("private/free-form detail admitted", operation)
		}
	}
	if len(reportOperations) > 256 {
		t.Fatal("report operation byte overflow", len(reportOperations))
	}
}
