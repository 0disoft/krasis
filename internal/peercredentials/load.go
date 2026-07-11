package peercredentials

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

func Load(caPath, certificatePath, privateKeyPath string) (tls.Certificate, *x509.CertPool, error) {
	if caPath == "" || certificatePath == "" || privateKeyPath == "" {
		return tls.Certificate{}, nil, errors.New("CA, certificate, and private key paths are required")
	}
	identity, err := tls.LoadX509KeyPair(certificatePath, privateKeyPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("load identity: %w", err)
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("read CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return tls.Certificate{}, nil, errors.New("CA file contains no certificates")
	}
	return identity, roots, nil
}
