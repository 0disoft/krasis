package peercredentials

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

func LoadTrustedPeer(path string) (transport.AuthenticatedPeerContext, error) {
	return loadTrustedPeerAt(path, time.Now())
}

func loadTrustedPeerAt(path string, now time.Time) (transport.AuthenticatedPeerContext, error) {
	if path == "" {
		return transport.AuthenticatedPeerContext{}, errors.New("trusted peer certificate path is required")
	}
	if now.IsZero() {
		return transport.AuthenticatedPeerContext{}, errors.New("trusted peer validation time is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return transport.AuthenticatedPeerContext{}, fmt.Errorf("read trusted peer certificate: %w", err)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(rest) != 0 {
		return transport.AuthenticatedPeerContext{}, errors.New("trusted peer file must contain exactly one PEM certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return transport.AuthenticatedPeerContext{}, fmt.Errorf("parse trusted peer certificate: %w", err)
	}
	if err := validateCertificateTime(certificate, now); err != nil {
		return transport.AuthenticatedPeerContext{}, fmt.Errorf("validate trusted peer certificate: %w", err)
	}
	peer, err := transport.AuthenticatedPeerFromCertificate(certificate)
	if err != nil {
		return transport.AuthenticatedPeerContext{}, fmt.Errorf("derive trusted peer identity: %w", err)
	}
	return peer, nil
}

var (
	ErrCertificateNotYetValid          = errors.New("certificate is not yet valid")
	ErrCertificateExpired              = errors.New("certificate has expired")
	ErrCertificateLifetimeInsufficient = errors.New("certificate expires before required time")
)

func validateCertificateTime(certificate *x509.Certificate, now time.Time) error {
	if certificate == nil {
		return errors.New("certificate is required")
	}
	if now.Before(certificate.NotBefore) {
		return fmt.Errorf("%w: not before %s", ErrCertificateNotYetValid, certificate.NotBefore.UTC().Format(time.RFC3339))
	}
	if now.After(certificate.NotAfter) {
		return fmt.Errorf("%w: not after %s", ErrCertificateExpired, certificate.NotAfter.UTC().Format(time.RFC3339))
	}
	return nil
}
