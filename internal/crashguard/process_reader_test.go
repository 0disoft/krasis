package crashguard

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

type countedProcessReader struct {
	bytes.Reader
	reads int
}

func (r *countedProcessReader) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}

func bufferedProcessFixture(t *testing.T, wire []byte) (*Process, *countedProcessReader) {
	t.Helper()
	reader := &countedProcessReader{Reader: *bytes.NewReader(wire)}
	writer := &closeTestPipe{}
	client, err := NewClient(writer, reader)
	if err != nil {
		t.Fatal(err)
	}
	return &Process{client: client, stdin: writer, stdout: io.NopCloser(reader), portable: true}, reader
}

func TestProcessInputACKHeaderAndSuffixUseOneUnderlyingRead(t *testing.T) {
	payload := bytes.Repeat([]byte{0x42}, 48)
	process, source := bufferedProcessFixture(t, payload)
	err := process.ExchangeInput(context.Background(), func(writer io.Writer, reader io.Reader) error {
		if _, err := writer.Write([]byte("input")); err != nil {
			return err
		}
		var header [24]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return err
		}
		var suffix [24]byte
		_, err := io.ReadFull(reader, suffix[:])
		return err
	})
	if err != nil || source.reads != 1 {
		t.Fatal("one complete native ACK required multiple underlying reads", source.reads, err)
	}
}

func TestProcessInputAndGuardLedgerSharePrefetchedResponseBytes(t *testing.T) {
	var wire bytes.Buffer
	if err := writeAck(&wire, operationPreparePress, statusOK); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0x42}, 48)
	wire.Write(payload)
	if err := writeAck(&wire, operationReleased, statusOK); err != nil {
		t.Fatal(err)
	}
	process, source := bufferedProcessFixture(t, wire.Bytes())
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 0x1e}
	if prepared, err := process.PreparePress(token); err != nil || !prepared {
		t.Fatal("guard preparation failed", prepared, err)
	}
	var received [48]byte
	if err := process.ExchangeInput(context.Background(), func(_ io.Writer, reader io.Reader) error {
		_, err := io.ReadFull(reader, received[:])
		return err
	}); err != nil || !bytes.Equal(received[:], payload) {
		t.Fatal("input exchange lost prefetched bytes", err)
	}
	if err := process.Released(token); err != nil {
		t.Fatal("input exchange consumed/lost the guard release ACK", err)
	}
	if source.reads != 1 {
		t.Fatal("ledger and input used separate response readers", source.reads)
	}
}
