package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
)

type PeerConnection struct {
	connection *tls.Conn
	reader     *Reader
	writer     *Writer
	writeMu    sync.Mutex
}

func NewPeerConnection(ctx context.Context, connection *tls.Conn) (*PeerConnection, error) {
	if connection == nil {
		return nil, errors.New("TLS connection is required")
	}
	if err := AuthenticatePeer(ctx, connection); err != nil {
		return nil, err
	}
	reader, err := NewReader(connection)
	if err != nil {
		return nil, err
	}
	writer, err := NewWriter(connection)
	if err != nil {
		return nil, err
	}
	return &PeerConnection{connection: connection, reader: reader, writer: writer}, nil
}

func (connection *PeerConnection) ReadFrame() (Frame, error) {
	if connection == nil || connection.reader == nil {
		return Frame{}, errors.New("peer connection is not initialized")
	}
	return connection.reader.ReadFrame()
}

func (connection *PeerConnection) WriteFrame(frame Frame) error {
	if connection == nil || connection.writer == nil {
		return errors.New("peer connection is not initialized")
	}
	connection.writeMu.Lock()
	defer connection.writeMu.Unlock()
	return connection.writer.WriteFrame(frame)
}

func (connection *PeerConnection) Close() error {
	if connection == nil || connection.connection == nil {
		return nil
	}
	return connection.connection.Close()
}
