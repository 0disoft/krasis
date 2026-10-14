package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestSourceReaderPreservesValidationAndExternalReadFailures(t *testing.T) {
	binding := Binding{7}
	for _, tc := range []struct {
		name      string
		packet    sourcePacket
		authority error
		want      error
	}{
		{"valid", sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 2, binding: binding}, nil, nil},
		{"wrong binding", sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 2, binding: Binding{8}}, nil, ErrProtocol},
		{"wrong sequence", sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 3, binding: binding}, nil, ErrProtocol},
		{"wrong kind", sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: 1, sequence: 2, binding: binding}, nil, ErrProtocol},
		{"authority lost", sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 2, binding: binding}, ErrAuthority, ErrAuthority},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := encodeSourcePacket(tc.packet)
			if err != nil {
				t.Fatal(err)
			}
			p, err := readSourceCommand(bytes.NewReader(data[:]), context.Background(), binding, 1, func() error { return tc.authority })
			if !errors.Is(err, tc.want) {
				t.Fatal("command result lost", err)
			}
			if tc.want == nil && p != tc.packet {
				t.Fatal("command changed")
			}
		})
	}
	local, cancel := context.WithCancel(context.Background())
	cancel()
	for _, cause := range []error{io.EOF, io.ErrUnexpectedEOF, os.ErrDeadlineExceeded, ErrProtocol} {
		_, result := readSourceCommand(sourceContextReader{ctx: local, read: func(context.Context, []byte) (int, error) { return 0, cause }}, local, binding, 1, func() error { return nil })
		if !errors.Is(result, cause) || errors.Is(result, context.Canceled) {
			t.Fatal("external failure became local stop", result)
		}
		final := sourceCaptureFinal(result, inputdesktop.ErrInactive, context.Canceled, false)
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{}, final); err != nil {
			t.Fatal(err)
		}
		if wire.Bytes()[5] != 1 {
			t.Fatal("joined external failure became success or service recovery", wire.Bytes()[5], final)
		}
		_, received := readReport(&wire)
		if received == nil || serviceRecoveryFailure(received, nil) != nil {
			t.Fatal("source read failure lost or clean retirement fenced service", received)
		}
	}
	_, result := readSourceCommand(sourceContextReader{ctx: local, read: func(context.Context, []byte) (int, error) { return 0, context.Canceled }}, local, binding, 1, func() error { return nil })
	if result != context.Canceled {
		t.Fatal("local read cancellation not distinguished", result)
	}
	if final := sourceCaptureFinal(result, inputdesktop.ErrInactive, context.Canceled, false); final != nil {
		t.Fatal("clean read stop became failure", final)
	}
}

func TestSourceCommandPumpReturnsTerminalErrorAndStopsOnlyAfterStop(t *testing.T) {
	for _, terminal := range []error{io.EOF, ErrProtocol, ErrAuthority} {
		run, cancel := context.WithCancel(context.Background())
		requests := make(chan sourcePacket, 1)
		reads := 0
		result := pumpSourceCommands(run, func() (sourcePacket, error) {
			reads++
			if reads == 1 {
				return sourcePacket{op: sourceHeartbeat, sequence: 1}, nil
			}
			return sourcePacket{}, terminal
		}, requests, cancel)
		if result != terminal || run.Err() != context.Canceled || (<-requests).sequence != 1 {
			t.Fatal("pump erased read failure", result)
		}
		cancel()
	}
	run, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan sourcePacket, 1)
	reads := 0
	if err := pumpSourceCommands(run, func() (sourcePacket, error) { reads++; return sourcePacket{op: sourceStop}, nil }, requests, cancel); err != nil || reads != 1 || run.Err() != nil || (<-requests).op != sourceStop {
		t.Fatal("explicit stop read beyond command or cancelled early", err)
	}
}
