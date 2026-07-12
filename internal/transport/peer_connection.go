package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"time"
)

type PeerConnection struct {
	connection *tls.Conn
	reader     *Reader
	writer     *Writer
	writeGate  chan struct{}
	peer       AuthenticatedPeerContext
}

func NewPeerConnection(ctx context.Context, connection *tls.Conn) (*PeerConnection, error) {
	if connection == nil {
		return nil, errors.New("TLS connection is required")
	}
	peer, err := AuthenticatePeerContext(ctx, connection)
	if err != nil {
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
	writeGate := make(chan struct{}, 1)
	writeGate <- struct{}{}
	return &PeerConnection{connection: connection, reader: reader, writer: writer, writeGate: writeGate, peer: peer}, nil
}

func (connection *PeerConnection) AuthenticatedPeer() (AuthenticatedPeerContext, error) {
	if connection == nil {
		return AuthenticatedPeerContext{}, errors.New("peer connection is not initialized")
	}
	if err := connection.peer.Validate(); err != nil {
		return AuthenticatedPeerContext{}, err
	}
	return connection.peer, nil
}

func (connection *PeerConnection) ReadFrame() (Frame, error) {
	if connection == nil || connection.reader == nil {
		return Frame{}, errors.New("peer connection is not initialized")
	}
	return connection.reader.ReadFrame()
}

func (connection *PeerConnection) WriteFrame(frame Frame) error {
	return connection.WriteFrameContext(context.Background(), frame)
}

func (connection *PeerConnection) WriteFrameContext(ctx context.Context, frame Frame) error {
	if connection == nil || connection.writer == nil || connection.connection == nil || connection.writeGate == nil {
		return errors.New("peer connection is not initialized")
	}
	if ctx == nil {
		return errors.New("write context is required")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-connection.writeGate:
	}
	defer func() { connection.writeGate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		if err := connection.connection.SetWriteDeadline(deadline); err != nil {
			return err
		}
	}
	stopCancel := context.AfterFunc(ctx, func() {
		_ = connection.connection.SetWriteDeadline(time.Now())
	})
	err := connection.writer.WriteFrame(frame)
	if stopCancel() {
		_ = connection.connection.SetWriteDeadline(time.Time{})
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	return err
}

func (connection *PeerConnection) Close() error {
	if connection == nil || connection.connection == nil {
		return nil
	}
	return connection.connection.Close()
}
