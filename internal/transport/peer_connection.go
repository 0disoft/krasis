package transport

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"time"
)

type PeerConnection struct {
	connection      *tls.Conn
	reader          *Reader
	writer          *Writer
	writeGate       chan struct{}
	peer            AuthenticatedPeerContext
	diagnosticID    string
	keepAlivePolicy string
}

const FrameWriteTimeout = 2 * time.Second

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
	state := connection.ConnectionState()
	var diagnosticID string
	if material, err := state.ExportKeyingMaterial("krasis/diagnostic-correlation/v1", nil, 32); err == nil {
		digest := sha256.Sum256(material)
		clear(material)
		diagnosticID = hex.EncodeToString(digest[:])
	}
	return &PeerConnection{connection: connection, reader: reader, writer: writer, writeGate: writeGate, peer: peer, diagnosticID: diagnosticID, keepAlivePolicy: configureConnectionKeepAlive(connection.NetConn())}, nil
}

// DiagnosticID is a label-specific digest shared only by this TLS connection.
// It is neither an identity credential nor exported TLS key material.
func (connection *PeerConnection) DiagnosticID() string {
	if connection == nil {
		return ""
	}
	return connection.diagnosticID
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

// SetReadProgressObserver is configured before the session reader starts.
func (connection *PeerConnection) SetReadProgressObserver(observer func(int, time.Time)) {
	if connection != nil && connection.reader != nil {
		connection.reader.SetReadProgressObserver(observer)
	}
}

func (connection *PeerConnection) WriteFrameContext(ctx context.Context, frame Frame) error {
	started := time.Now()
	var admitted time.Time
	failure := func(phase string, cause error) error {
		value := &FrameWriteFailure{Phase: phase, Kind: frame.Kind, Cause: cause, Wait: time.Since(started)}
		if !admitted.IsZero() {
			value.Wait = admitted.Sub(started)
			value.IO = time.Since(admitted)
		}
		return value
	}
	if connection == nil || connection.writer == nil || connection.connection == nil || connection.writeGate == nil {
		return errors.New("peer connection is not initialized")
	}
	if ctx == nil {
		return errors.New("write context is required")
	}
	if err := ctx.Err(); err != nil {
		return failure("admission", err)
	}
	ctx, cancel := context.WithTimeout(ctx, FrameWriteTimeout)
	defer cancel()
	select {
	case <-ctx.Done():
		return failure("admission", ctx.Err())
	case <-connection.writeGate:
	}
	defer func() { connection.writeGate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return failure("admission", err)
	}
	admitted = time.Now()
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		if err := connection.connection.SetWriteDeadline(deadline); err != nil {
			_ = connection.connection.NetConn().Close()
			return failure("deadline", err)
		}
	}
	finishWrite := interruptWriteOnCancel(ctx, connection.connection)
	err := connection.writer.WriteFrame(frame)
	finishWrite()
	if err != nil {
		// Failed TLS writes cannot be reused. Close the socket directly rather
		// than spending another TLS close-notify timeout on an unusable stream.
		_ = connection.connection.NetConn().Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return failure("io", errors.Join(ctxErr, err))
		}
		return failure("io", err)
	}
	return err
}

// The caller holds writeGate until this cleanup returns. AfterFunc's stop alone
// does not join a callback that has already started.
func interruptWriteOnCancel(ctx context.Context, connection interface{ SetWriteDeadline(time.Time) error }) func() {
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		_ = connection.SetWriteDeadline(time.Now())
	})
	return func() {
		if !stop() {
			<-done
		}
		_ = connection.SetWriteDeadline(time.Time{})
	}
}

func (connection *PeerConnection) Close() error {
	if connection == nil || connection.connection == nil {
		return nil
	}
	return connection.connection.Close()
}
