package peercredentials

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/0disoft/krasis/internal/transport"
)

func LoadTrustedPeer(path string) (transport.AuthenticatedPeerContext, error) {
	if path == "" {
		return transport.AuthenticatedPeerContext{}, errors.New("trusted peer certificate path is required")
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
	peer, err := transport.AuthenticatedPeerFromCertificate(certificate)
	if err != nil {
		return transport.AuthenticatedPeerContext{}, fmt.Errorf("derive trusted peer identity: %w", err)
	}
	return peer, nil
}
