package transport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	ProtocolVersion uint8  = 1
	MaxPayloadSize  uint32 = 64 * 1024
	headerSize             = 12
)

var (
	frameMagic          = [4]byte{'K', 'R', 'A', 'S'}
	ErrInvalidFrame     = errors.New("invalid peer frame")
	ErrUnsupportedFrame = errors.New("unsupported peer frame")
	ErrPayloadTooLarge  = errors.New("peer frame payload too large")
)

type MessageKind uint8

const (
	MessageControl   MessageKind = 1
	MessageInput     MessageKind = 2
	MessageHeartbeat MessageKind = 3
)

type Frame struct {
	Kind    MessageKind
	Payload []byte
}

type Reader struct {
	source     io.Reader
	maxPayload uint32
}

func NewReader(source io.Reader) (*Reader, error) {
	if source == nil {
		return nil, errors.New("frame source is required")
	}
	return &Reader{source: source, maxPayload: MaxPayloadSize}, nil
}

func (reader *Reader) ReadFrame() (Frame, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(reader.source, header[:]); errors.Is(err, io.EOF) {
		return Frame{}, io.EOF
	} else if err != nil {
		return Frame{}, fmt.Errorf("%w: read header: %w", ErrInvalidFrame, err)
	}
	if [4]byte(header[0:4]) != frameMagic {
		return Frame{}, fmt.Errorf("%w: magic does not match", ErrInvalidFrame)
	}
	if header[4] != ProtocolVersion {
		return Frame{}, fmt.Errorf("%w: protocol version %d", ErrUnsupportedFrame, header[4])
	}
	kind := MessageKind(header[5])
	if !kind.Valid() {
		return Frame{}, fmt.Errorf("%w: message kind %d", ErrUnsupportedFrame, kind)
	}
	if binary.BigEndian.Uint16(header[6:8]) != 0 {
		return Frame{}, fmt.Errorf("%w: reserved flags are not zero", ErrUnsupportedFrame)
	}
	payloadSize := binary.BigEndian.Uint32(header[8:12])
	if payloadSize > reader.maxPayload {
		return Frame{}, fmt.Errorf("%w: declared=%d maximum=%d", ErrPayloadTooLarge, payloadSize, reader.maxPayload)
	}
	payload := make([]byte, payloadSize)
	if _, err := io.ReadFull(reader.source, payload); err != nil {
		return Frame{}, fmt.Errorf("%w: read payload: %w", ErrInvalidFrame, err)
	}
	return Frame{Kind: kind, Payload: payload}, nil
}

// Writer requires serialized calls, as provided by PeerConnection's write gate.
type Writer struct {
	destination io.Writer
	maxPayload  uint32
	buffer      []byte
}

func NewWriter(destination io.Writer) (*Writer, error) {
	if destination == nil {
		return nil, errors.New("frame destination is required")
	}
	return &Writer{destination: destination, maxPayload: MaxPayloadSize}, nil
}

func (writer *Writer) WriteFrame(frame Frame) error {
	if !frame.Kind.Valid() {
		return fmt.Errorf("%w: message kind %d", ErrUnsupportedFrame, frame.Kind)
	}
	if uint64(len(frame.Payload)) > uint64(writer.maxPayload) {
		return fmt.Errorf("%w: actual=%d maximum=%d", ErrPayloadTooLarge, len(frame.Payload), writer.maxPayload)
	}
	size := headerSize + len(frame.Payload)
	if cap(writer.buffer) < size {
		writer.buffer = make([]byte, size)
	}
	packet := writer.buffer[:size]
	header := packet[:headerSize]
	clear(header)
	copy(header[0:4], frameMagic[:])
	header[4] = ProtocolVersion
	header[5] = byte(frame.Kind)
	binary.BigEndian.PutUint32(header[8:12], uint32(len(frame.Payload)))
	copy(packet[headerSize:], frame.Payload)
	if err := writeFull(writer.destination, packet); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return nil
}

func (kind MessageKind) Valid() bool {
	switch kind {
	case MessageControl, MessageInput, MessageHeartbeat:
		return true
	default:
		return false
	}
}

func writeFull(destination io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := destination.Write(data)
		if err != nil {
			return err
		}
		if written <= 0 || written > len(data) {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}
