package transport

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"time"
)

// A paired leaf is a persisted key anchor, not an indefinitely valid on-wire
// certificate. Only the explicit product key-pinning path permits renewal.
const ProductCertificateLifetime = 30 * 24 * time.Hour

func ProductKeyFingerprint(c *x509.Certificate) (string, error) {
	if err := ValidateProductKeyAnchor(c); err != nil {
		return "", err
	}
	digest := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(digest[:]), nil
}

func ValidateProductKeyAnchor(c *x509.Certificate) error {
	if c == nil || len(c.Raw) == 0 || c.IsCA || c.KeyUsage != x509.KeyUsageDigitalSignature ||
		!c.BasicConstraintsValid || c.NotBefore.IsZero() || !c.NotAfter.After(c.NotBefore) ||
		c.NotAfter.Sub(c.NotBefore) > ProductCertificateLifetime+time.Minute ||
		len(c.DNSNames) != 1 || c.DNSNames[0] != "krasis.invalid" || len(c.ExtKeyUsage) != 1 || len(c.UnhandledCriticalExtensions) != 0 {
		return ErrPeerAuthentication
	}
	if _, ok := c.PublicKey.(ed25519.PublicKey); !ok {
		return ErrPeerAuthentication
	}
	if _, err := AuthenticatedPeerFromCertificate(c); err != nil {
		return ErrPeerAuthentication
	}
	return c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature)
}

// AuthenticateProductKey keeps the original pairing fingerprint as the policy
// principal. The live leaf must independently prove the paired key, exact role,
// self-signature and current bounded validity. Never accept an expired wire leaf.
func AuthenticateProductKey(state tls.ConnectionState, anchor *x509.Certificate, now time.Time) (AuthenticatedPeerContext, error) {
	if now.IsZero() || state.Version != tls.VersionTLS13 || state.DidResume || len(state.PeerCertificates) != 1 || ValidateProductKeyAnchor(anchor) != nil {
		return AuthenticatedPeerContext{}, ErrPeerAuthentication
	}
	live := state.PeerCertificates[0]
	paired, err := AuthenticatedPeerFromCertificate(anchor)
	if err != nil || ValidateProductKeyAnchor(live) != nil || now.Before(live.NotBefore) || !now.Before(live.NotAfter) ||
		!bytes.Equal(live.RawSubjectPublicKeyInfo, anchor.RawSubjectPublicKeyInfo) || authenticatedPeerRole(live) != paired.Role {
		return AuthenticatedPeerContext{}, ErrPeerAuthentication
	}
	return paired, nil
}

// NewProductKeyTLSConfig performs full explicit key authentication instead of
// x509 chain validation against a short-lived self-signed leaf. Legacy CA TLS
// constructors retain their existing verified-chain requirement.
func NewProductKeyTLSConfig(local func() (tls.Certificate, error), anchor *x509.Certificate, listener bool, authorize func() error) (*tls.Config, error) {
	if local == nil || authorize == nil || ValidateProductKeyAnchor(anchor) != nil {
		return nil, ErrInvalidTLSConfig
	}
	paired, err := AuthenticatedPeerFromCertificate(anchor)
	if err != nil || listener != (paired.Role == AuthenticatedPeerConnector) {
		return nil, ErrInvalidTLSConfig
	}
	// Freeze the anchor against mutation by callers after construction.
	anchor, err = x509.ParseCertificate(bytes.Clone(anchor.Raw))
	if err != nil {
		return nil, ErrInvalidTLSConfig
	}
	identity := func() (*tls.Certificate, error) {
		if err := authorize(); err != nil {
			return nil, err
		}
		c, err := local()
		if err != nil || validateIdentity(c) != nil || ValidateProductKeyAnchor(c.Leaf) != nil ||
			time.Now().Before(c.Leaf.NotBefore) || !time.Now().Before(c.Leaf.NotAfter) ||
			(authenticatedPeerRole(c.Leaf) == AuthenticatedPeerListener) != listener {
			return nil, ErrInvalidTLSConfig
		}
		return &c, nil
	}
	config := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		NextProtos: []string{ALPNProtocol}, SessionTicketsDisabled: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			if _, err := AuthenticateProductKey(state, anchor, time.Now()); err != nil {
				return err
			}
			return authorize()
		}}
	if listener {
		config.ClientAuth = tls.RequireAnyClientCert
		config.GetCertificate = func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return identity() }
	} else {
		// The mandatory VerifyConnection above replaces chain/name verification
		// with the exact paired product key/role/name/signature/validity checks.
		config.InsecureSkipVerify = true
		config.ServerName = "krasis.invalid"
		config.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return identity() }
	}
	return config, nil
}

func productKeyAnchor(anchors []*x509.Certificate) (*x509.Certificate, error) {
	if len(anchors) == 0 {
		return nil, nil
	}
	if len(anchors) != 1 || ValidateProductKeyAnchor(anchors[0]) != nil {
		return nil, errors.New("one valid product key anchor is required")
	}
	return x509.ParseCertificate(bytes.Clone(anchors[0].Raw))
}

// CloneProductKeyAnchor validates/fixes an explicit anchor for a network source.
func CloneProductKeyAnchor(anchors ...*x509.Certificate) (*x509.Certificate, error) {
	return productKeyAnchor(anchors)
}
