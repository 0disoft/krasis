package peercredentials

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"
)

func Load(caPath, certificatePath, privateKeyPath string) (tls.Certificate, *x509.CertPool, error) {
	return loadAt(caPath, certificatePath, privateKeyPath, time.Now())
}

func ValidateIdentityValidThrough(identity tls.Certificate, roots *x509.CertPool, requiredThrough time.Time) error {
	if identity.Leaf == nil {
		return errors.New("parsed identity certificate is required")
	}
	if roots == nil {
		return errors.New("identity root pool is required")
	}
	if requiredThrough.IsZero() {
		return errors.New("required certificate validity time is required")
	}
	intermediates := x509.NewCertPool()
	for _, encoded := range identity.Certificate[1:] {
		certificate, err := x509.ParseCertificate(encoded)
		if err != nil {
			return fmt.Errorf("parse identity intermediate certificate: %w", err)
		}
		intermediates.AddCert(certificate)
	}
	if _, err := identity.Leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates, CurrentTime: requiredThrough,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return fmt.Errorf(
			"%w at %s: %v",
			ErrCertificateLifetimeInsufficient,
			requiredThrough.UTC().Format(time.RFC3339),
			err,
		)
	}
	return nil
}

func loadAt(caPath, certificatePath, privateKeyPath string, now time.Time) (tls.Certificate, *x509.CertPool, error) {
	if caPath == "" || certificatePath == "" || privateKeyPath == "" {
		return tls.Certificate{}, nil, errors.New("CA, certificate, and private key paths are required")
	}
	if now.IsZero() {
		return tls.Certificate{}, nil, errors.New("credential validation time is required")
	}
	if err := ValidatePrivateKeySecurity(privateKeyPath); err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("validate private key security: %w", err)
	}
	identity, err := tls.LoadX509KeyPair(certificatePath, privateKeyPath)
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("load identity: %w", err)
	}
	if len(identity.Certificate) == 0 {
		return tls.Certificate{}, nil, errors.New("identity contains no certificates")
	}
	leaf, err := x509.ParseCertificate(identity.Certificate[0])
	if err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("parse identity certificate: %w", err)
	}
	if err := validateCertificateTime(leaf, now); err != nil {
		return tls.Certificate{}, nil, fmt.Errorf("validate identity certificate: %w", err)
	}
	identity.Leaf = leaf
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
