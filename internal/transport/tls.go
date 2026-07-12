package transport

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
)

const ALPNProtocol = "krasis/1"

type AuthenticatedPeerRole string

const (
	AuthenticatedPeerListener  AuthenticatedPeerRole = "LISTENER"
	AuthenticatedPeerConnector AuthenticatedPeerRole = "CONNECTOR"
)

type AuthenticatedPeerContext struct {
	CertificateSHA256 string
	Role              AuthenticatedPeerRole
}

func (peer AuthenticatedPeerContext) Validate() error {
	if len(peer.CertificateSHA256) != sha256.Size*2 {
		return errors.New("authenticated peer certificate fingerprint is invalid")
	}
	if _, err := hex.DecodeString(peer.CertificateSHA256); err != nil {
		return errors.New("authenticated peer certificate fingerprint is invalid")
	}
	if peer.Role != AuthenticatedPeerListener && peer.Role != AuthenticatedPeerConnector {
		return errors.New("authenticated peer role is invalid")
	}
	return nil
}

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
	_, err := AuthenticatePeerContext(ctx, connection)
	return err
}

func AuthenticatePeerContext(ctx context.Context, connection *tls.Conn) (AuthenticatedPeerContext, error) {
	if ctx == nil || connection == nil {
		return AuthenticatedPeerContext{}, errors.Join(ErrPeerAuthentication, errors.New("context and TLS connection are required"))
	}
	if err := connection.HandshakeContext(ctx); err != nil {
		return AuthenticatedPeerContext{}, fmt.Errorf("%w: %w", ErrPeerAuthentication, err)
	}
	state := connection.ConnectionState()
	if state.Version != tls.VersionTLS13 {
		return AuthenticatedPeerContext{}, fmt.Errorf("%w: negotiated TLS version %x", ErrPeerAuthentication, state.Version)
	}
	if state.NegotiatedProtocol != ALPNProtocol {
		return AuthenticatedPeerContext{}, fmt.Errorf("%w: negotiated protocol %q", ErrPeerAuthentication, state.NegotiatedProtocol)
	}
	if len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return AuthenticatedPeerContext{}, errors.Join(ErrPeerAuthentication, errors.New("verified peer certificate chain is required"))
	}
	certificate := state.PeerCertificates[0]
	fingerprint := sha256.Sum256(certificate.Raw)
	peer := AuthenticatedPeerContext{
		CertificateSHA256: hex.EncodeToString(fingerprint[:]),
		Role:              authenticatedPeerRole(certificate),
	}
	if err := peer.Validate(); err != nil {
		return AuthenticatedPeerContext{}, errors.Join(ErrPeerAuthentication, err)
	}
	return peer, nil
}

func authenticatedPeerRole(certificate *x509.Certificate) AuthenticatedPeerRole {
	if certificate == nil {
		return ""
	}
	var listener, connector bool
	for _, usage := range certificate.ExtKeyUsage {
		switch usage {
		case x509.ExtKeyUsageServerAuth:
			listener = true
		case x509.ExtKeyUsageClientAuth:
			connector = true
		}
	}
	if listener == connector {
		return ""
	}
	if listener {
		return AuthenticatedPeerListener
	}
	if connector {
		return AuthenticatedPeerConnector
	}
	return ""
}

func validateIdentity(identity tls.Certificate) error {
	if len(identity.Certificate) == 0 || identity.PrivateKey == nil {
		return errors.Join(ErrInvalidTLSConfig, errors.New("certificate chain and private key are required"))
	}
	return nil
}
