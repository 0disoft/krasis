package transport

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	frames := []Frame{
		{Kind: MessageControl, Payload: []byte("lease")},
		{Kind: MessageInput, Payload: []byte{0, 1, 2, 3}},
		{Kind: MessageHeartbeat},
	}
	var wire bytes.Buffer
	writer, err := NewWriter(&wire)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	for _, frame := range frames {
		if err := writer.WriteFrame(frame); err != nil {
			t.Fatalf("WriteFrame() error = %v", err)
		}
	}
	reader, err := NewReader(&wire)
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}
	for index, want := range frames {
		got, err := reader.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame(%d) error = %v", index, err)
		}
		if got.Kind != want.Kind || !bytes.Equal(got.Payload, want.Payload) {
			t.Fatalf("ReadFrame(%d) = %#v, want %#v", index, got, want)
		}
	}
}

func TestFrameReaderHandlesFragmentedInput(t *testing.T) {
	var wire bytes.Buffer
	writer, _ := NewWriter(&wire)
	want := Frame{Kind: MessageInput, Payload: []byte("fragmented")}
	if err := writer.WriteFrame(want); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	reader, _ := NewReader(oneByteReader{source: bytes.NewReader(wire.Bytes())})
	got, err := reader.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if got.Kind != want.Kind || !bytes.Equal(got.Payload, want.Payload) {
		t.Fatalf("ReadFrame() = %#v, want %#v", got, want)
	}
}

func TestFrameRejectsInvalidHeaderBeforePayloadAllocation(t *testing.T) {
	tests := []struct {
		name string
		wire []byte
		want error
	}{
		{name: "magic", wire: headerWith('X', ProtocolVersion, byte(MessageInput), 0, 0), want: ErrInvalidFrame},
		{name: "version", wire: headerWith('K', ProtocolVersion+1, byte(MessageInput), 0, 0), want: ErrUnsupportedFrame},
		{name: "kind", wire: headerWith('K', ProtocolVersion, 99, 0, 0), want: ErrUnsupportedFrame},
		{name: "flags", wire: headerWith('K', ProtocolVersion, byte(MessageInput), 1, 0), want: ErrUnsupportedFrame},
		{name: "oversized", wire: headerWith('K', ProtocolVersion, byte(MessageInput), 0, MaxPayloadSize+1), want: ErrPayloadTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader, _ := NewReader(bytes.NewReader(test.wire))
			_, err := reader.ReadFrame()
			if !errors.Is(err, test.want) {
				t.Fatalf("ReadFrame() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestFrameRejectsTruncatedPayload(t *testing.T) {
	wire := append(headerWith('K', ProtocolVersion, byte(MessageInput), 0, 3), 1, 2)
	reader, _ := NewReader(bytes.NewReader(wire))
	_, err := reader.ReadFrame()
	if !errors.Is(err, ErrInvalidFrame) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadFrame() error = %v", err)
	}
}

func TestFrameReaderDistinguishesCleanCloseFromTruncatedHeader(t *testing.T) {
	reader, _ := NewReader(bytes.NewReader(nil))
	_, err := reader.ReadFrame()
	if !errors.Is(err, io.EOF) || errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("clean close error = %v, want only io.EOF", err)
	}

	reader, _ = NewReader(bytes.NewReader(frameMagic[:2]))
	_, err = reader.ReadFrame()
	if !errors.Is(err, io.ErrUnexpectedEOF) || !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("truncated header error = %v", err)
	}
}

func TestFrameWriterRejectsInvalidOrOversizedFrame(t *testing.T) {
	writer, _ := NewWriter(io.Discard)
	if err := writer.WriteFrame(Frame{Kind: 99}); !errors.Is(err, ErrUnsupportedFrame) {
		t.Fatalf("invalid kind error = %v", err)
	}
	if err := writer.WriteFrame(Frame{Kind: MessageInput, Payload: make([]byte, MaxPayloadSize+1)}); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("oversized payload error = %v", err)
	}
}

func TestFrameNetPipePreservesOrder(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	writer, _ := NewWriter(left)
	reader, _ := NewReader(right)
	want := []Frame{
		{Kind: MessageControl, Payload: []byte("grant")},
		{Kind: MessageInput, Payload: []byte("down")},
		{Kind: MessageHeartbeat},
	}
	writeDone := make(chan error, 1)
	go func() {
		for _, frame := range want {
			if err := writer.WriteFrame(frame); err != nil {
				writeDone <- err
				return
			}
		}
		writeDone <- nil
	}()
	for index, expected := range want {
		got, err := reader.ReadFrame()
		if err != nil {
			t.Fatalf("ReadFrame(%d) error = %v", index, err)
		}
		if got.Kind != expected.Kind || !bytes.Equal(got.Payload, expected.Payload) {
			t.Fatalf("ReadFrame(%d) = %#v, want %#v", index, got, expected)
		}
	}
	if err := <-writeDone; err != nil {
		t.Fatalf("writer error = %v", err)
	}
}

func headerWith(magicFirst byte, version, kind byte, flags uint16, payloadSize uint32) []byte {
	header := make([]byte, headerSize)
	copy(header[0:4], frameMagic[:])
	header[0] = magicFirst
	header[4] = version
	header[5] = kind
	binary.BigEndian.PutUint16(header[6:8], flags)
	binary.BigEndian.PutUint32(header[8:12], payloadSize)
	return header
}

type oneByteReader struct {
	source io.Reader
}

func (reader oneByteReader) Read(target []byte) (int, error) {
	if len(target) > 1 {
		target = target[:1]
	}
	return reader.source.Read(target)
}
