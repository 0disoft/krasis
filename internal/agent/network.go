package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

type ListenSource struct {
	listener  net.Listener
	tlsConfig *tls.Config
	closeOnce sync.Once
	closeErr  error
}

func NewListenSource(address string, tlsConfig *tls.Config) (*ListenSource, error) {
	if address == "" || tlsConfig == nil {
		return nil, errors.New("listen address and TLS config are required")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	return &ListenSource{listener: listener, tlsConfig: tlsConfig.Clone()}, nil
}

func (source *ListenSource) Address() string {
	if source == nil || source.listener == nil {
		return ""
	}
	return source.listener.Addr().String()
}

func (source *ListenSource) Next(ctx context.Context) (Connection, error) {
	if source == nil || source.listener == nil || source.tlsConfig == nil {
		return nil, errors.New("listen source is not initialized")
	}
	stop := context.AfterFunc(ctx, func() { _ = source.Close() })
	defer stop()
	connection, err := source.listener.Accept()
	if err != nil {
		return nil, err
	}
	peerConnection, err := transport.NewPeerConnection(ctx, tls.Server(connection, source.tlsConfig.Clone()))
	if err != nil {
		return nil, closeConnectionOnError(connection, err)
	}
	return peerConnection, nil
}

func (source *ListenSource) Close() error {
	if source == nil || source.listener == nil {
		return nil
	}
	source.closeOnce.Do(func() { source.closeErr = source.listener.Close() })
	return source.closeErr
}

type DialSource struct {
	address     string
	tlsConfig   *tls.Config
	dialTimeout time.Duration
}

func NewDialSource(address string, tlsConfig *tls.Config, dialTimeout time.Duration) (*DialSource, error) {
	if address == "" || tlsConfig == nil || dialTimeout <= 0 {
		return nil, errors.New("dial address, TLS config, and positive timeout are required")
	}
	return &DialSource{address: address, tlsConfig: tlsConfig.Clone(), dialTimeout: dialTimeout}, nil
}

func (source *DialSource) Next(ctx context.Context) (Connection, error) {
	if source == nil || source.address == "" || source.tlsConfig == nil || source.dialTimeout <= 0 {
		return nil, errors.New("dial source is not initialized")
	}
	dialer := net.Dialer{Timeout: source.dialTimeout}
	connection, err := dialer.DialContext(ctx, "tcp", source.address)
	if err != nil {
		return nil, err
	}
	peerConnection, err := transport.NewPeerConnection(ctx, tls.Client(connection, source.tlsConfig.Clone()))
	if err != nil {
		return nil, closeConnectionOnError(connection, err)
	}
	return peerConnection, nil
}

func (*DialSource) Close() error { return nil }

func closeConnectionOnError(connection net.Conn, cause error) error {
	if connection == nil {
		return cause
	}
	return errors.Join(cause, connection.Close())
}
