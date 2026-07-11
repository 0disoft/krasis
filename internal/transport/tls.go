package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
)

const ALPNProtocol = "krasis/1"

var (
	ErrInvalidTLSConfig   = errors.New("invalid peer TLS configuration")
	ErrPeerAuthentication = errors.New("peer TLS authentication failed")
)

func NewServerTLSConfig(identity tls.Certificate, clientRoots *x509.CertPool) (*tls.Config, error) {
	if err := validateIdentity(identity); err != nil {
		return nil, err
	}
	if clientRoots == nil {
		return nil, errors.Join(ErrInvalidTLSConfig, errors.New("client trust roots are required"))
	}
	return &tls.Config{
		Certificates: []tls.Certificate{identity},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientRoots.Clone(),
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{ALPNProtocol},
	}, nil
}

func NewClientTLSConfig(
	identity tls.Certificate,
	serverRoots *x509.CertPool,
	expectedServerName string,
) (*tls.Config, error) {
	if err := validateIdentity(identity); err != nil {
		return nil, err
	}
	if serverRoots == nil {
		return nil, errors.Join(ErrInvalidTLSConfig, errors.New("server trust roots are required"))
	}
	if expectedServerName == "" {
		return nil, errors.Join(ErrInvalidTLSConfig, errors.New("expected server name is required"))
	}
	return &tls.Config{
		Certificates: []tls.Certificate{identity},
		RootCAs:      serverRoots.Clone(),
		ServerName:   expectedServerName,
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{ALPNProtocol},
	}, nil
}

func AuthenticatePeer(ctx context.Context, connection *tls.Conn) error {
	if ctx == nil || connection == nil {
		return errors.Join(ErrPeerAuthentication, errors.New("context and TLS connection are required"))
	}
	if err := connection.HandshakeContext(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrPeerAuthentication, err)
	}
	state := connection.ConnectionState()
	if state.Version != tls.VersionTLS13 {
		return fmt.Errorf("%w: negotiated TLS version %x", ErrPeerAuthentication, state.Version)
	}
	if state.NegotiatedProtocol != ALPNProtocol {
		return fmt.Errorf("%w: negotiated protocol %q", ErrPeerAuthentication, state.NegotiatedProtocol)
	}
	if len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return errors.Join(ErrPeerAuthentication, errors.New("verified peer certificate chain is required"))
	}
	return nil
}

func validateIdentity(identity tls.Certificate) error {
	if len(identity.Certificate) == 0 || identity.PrivateKey == nil {
		return errors.Join(ErrInvalidTLSConfig, errors.New("certificate chain and private key are required"))
	}
	return nil
}
