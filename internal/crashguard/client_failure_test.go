package crashguard

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

// A read failure does not prove that the peer did not process the command.
// The queued ACK identifies only the operation, not the pressed token.
type interruptedAck struct {
	io.Reader
	err error
}

func (reader *interruptedAck) Read(p []byte) (int, error) {
	if reader.err != nil {
		err := reader.err
		reader.err = nil
		return 0, err
	}
	return reader.Reader.Read(p)
}

func TestClientNeverReusesUnconfirmedAcknowledgement(t *testing.T) {
	var commands, pending bytes.Buffer
	if err := writeAck(&pending, operationPreparePress, statusOK); err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("ack read interrupted")
	client, err := NewClient(&commands, &interruptedAck{Reader: &pending, err: interrupted})
	if err != nil {
		t.Fatal(err)
	}
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	if prepared, err := client.PreparePress(key); prepared || !errors.Is(err, interrupted) {
		t.Fatal("missing ACK was accepted", prepared, err)
	}
	key.Code++
	if prepared, err := client.PreparePress(key); prepared || !errors.Is(err, interrupted) {
		t.Fatal("old ACK authorized a different key", prepared, err)
	}
	for _, err := range []error{client.Released(key), client.CancelPreparedPress(key), client.Disarm(), client.Disarm()} {
		if !errors.Is(err, interrupted) {
			t.Fatal("unconfirmed stream lost its failure", err)
		}
	}
	if commands.Len() != commandSize || pending.Len() != ackSize {
		t.Fatal("failed stream performed more IPC", commands.Len(), pending.Len())
	}
}

type incompleteCommandWriter struct {
	calls int
	err   error
}

func (writer *incompleteCommandWriter) Write(p []byte) (int, error) {
	writer.calls++
	return len(p) - 1, writer.err
}

func TestClientFencesIncompleteCommandWrites(t *testing.T) {
	for _, failure := range []error{nil, io.ErrClosedPipe} {
		t.Run("write_"+errors.Join(io.ErrShortWrite, failure).Error(), func(t *testing.T) {
			writer := &incompleteCommandWriter{err: failure}
			var pending bytes.Buffer
			_ = writeAck(&pending, operationPreparePress, statusOK)
			client, _ := NewClient(writer, &pending)
			key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
			want := failure
			if want == nil {
				want = io.ErrShortWrite
			}
			if prepared, err := client.PreparePress(key); prepared || !errors.Is(err, want) {
				t.Fatal("incomplete command was accepted", prepared, err)
			}
			if err := client.Disarm(); !errors.Is(err, want) || writer.calls != 1 || pending.Len() != ackSize {
				t.Fatal("incomplete command stream was reused", err, writer.calls, pending.Len())
			}
		})
	}
}

func TestClientDisarmRetainsRecoveryFailure(t *testing.T) {
	var commands, pending bytes.Buffer
	_ = writeAck(&pending, operationDisarm, statusRejected)
	client, _ := NewClient(&commands, &pending)
	for range 2 {
		if err := client.Disarm(); !errors.Is(err, ErrRecoveryFailed) {
			t.Fatal("failed recovery became successful", err)
		}
	}
	if commands.Len() != commandSize {
		t.Fatal("disarm was retried", commands.Len())
	}
}

func TestClientLocalValidationDoesNotFenceHealthyStream(t *testing.T) {
	var commands, pending bytes.Buffer
	_ = writeAck(&pending, operationPreparePress, statusOK)
	_ = writeAck(&pending, operationPreparePress, statusAlreadyPrepared)
	_ = writeAck(&pending, operationDisarm, statusOK)
	client, _ := NewClient(&commands, &pending)
	if _, err := client.PreparePress(control.PressedToken{}); err == nil || commands.Len() != 0 {
		t.Fatal("invalid token reached the wire", err)
	}
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	if prepared, err := client.PreparePress(key); !prepared || err != nil {
		t.Fatal("local rejection fenced a healthy stream", prepared, err)
	}
	if prepared, err := client.PreparePress(key); prepared || err != nil {
		t.Fatal("confirmed duplicate lost its semantics", prepared, err)
	}
	for range 2 {
		if err := client.Disarm(); err != nil {
			t.Fatal("confirmed disarm is not idempotent", err)
		}
	}
	if commands.Len() != 3*commandSize {
		t.Fatal("unexpected wire command count", commands.Len())
	}
}

func TestProcessCannotReuseFailedGuardStreamOrReportCleanClose(t *testing.T) {
	process := closeTestProcess(t)
	var commands, pending bytes.Buffer
	_ = writeAck(&pending, operationPreparePress, statusOK)
	interrupted := errors.New("ack read interrupted")
	process.client, _ = NewClient(&commands, &interruptedAck{Reader: &pending, err: interrupted})
	key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	if _, err := process.PreparePress(key); !errors.Is(err, interrupted) {
		t.Fatal("process lost the first failure", err)
	}
	key.Code++
	if prepared, err := process.PreparePress(key); prepared || !errors.Is(err, interrupted) {
		t.Fatal("process reused the failed guard stream", prepared, err)
	}
	close(process.done) // Fixture exit evidence, not native recovery evidence.
	if err := process.Close(); !errors.Is(err, interrupted) || commands.Len() != commandSize {
		t.Fatal("close lost uncertain recovery or reused the stream", err, commands.Len())
	}
}
