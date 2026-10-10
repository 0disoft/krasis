package productidentity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

const TransportRenewalLead = 7 * 24 * time.Hour

// RenewableTransport reissues a bounded leaf with the already paired role key.
// It never changes device identity, stored keys, pairing pins or Windows ACLs.
// Reissuing in memory also works before sign-in without service writes to the
// user's credential store. Both endpoints must support product key pinning.
func RenewableTransport(anchor tls.Certificate, now time.Time) (tls.Certificate, error) {
	if now.IsZero() || transport.ValidateProductKeyAnchor(anchor.Leaf) != nil || now.Before(anchor.Leaf.NotBefore) {
		return tls.Certificate{}, ErrTransportRenewalRequired
	}
	if now.Before(anchor.Leaf.NotAfter.Add(-TransportRenewalLead)) {
		return anchor, nil
	}
	key, ok := anchor.PrivateKey.(ed25519.PrivateKey)
	if !ok || len(key) != ed25519.PrivateKeySize || !bytes.Equal(key.Public().(ed25519.PublicKey), anchor.Leaf.PublicKey.(ed25519.PublicKey)) {
		return tls.Certificate{}, ErrTransportRenewalRequired
	}
	// Stable renewal windows avoid policy churn when the service observes its
	// grant once per second. TLS checks each presented leaf's actual validity.
	step := TransportLifetime - TransportRenewalLead
	first := anchor.Leaf.NotAfter.Add(-TransportRenewalLead)
	start := first.Add(time.Duration(now.Sub(first)/step) * step)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	serial.Add(serial, big.NewInt(1))
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Krasis product transport"},
		NotBefore: start.Add(-time.Minute), NotAfter: start.Add(TransportLifetime),
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: append([]x509.ExtKeyUsage(nil), anchor.Leaf.ExtKeyUsage...), DNSNames: []string{TransportServerName}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return tls.Certificate{}, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}

// OpenTransportAnchor reads an existing role key regardless of leaf expiry.
// This is not a TLS credential: callers must use RenewableTransport and the
// explicit product-key verifier; it cannot create or rotate missing keys.
func (store *Store) OpenTransportAnchor(role transport.AuthenticatedPeerRole) (tls.Certificate, error) {
	if role != transport.AuthenticatedPeerListener && role != transport.AuthenticatedPeerConnector {
		return tls.Certificate{}, ErrTransportRenewalRequired
	}
	dir, release, err := store.openTransportReadLocked()
	if err != nil {
		return tls.Certificate{}, err
	}
	defer release()
	identity, err := loadFromDirectory(dir)
	if err != nil {
		return tls.Certificate{}, err
	}
	doc, err := readTransport(dir, "transport-"+string(role)+".json")
	if err != nil {
		return tls.Certificate{}, err
	}
	return validateTransportMaterial(doc, identity.Public(), role)
}

func (store *Store) InspectRenewableTransport(role transport.AuthenticatedPeerRole, now time.Time) (TransportInfo, error) {
	anchor, err := store.OpenTransportAnchor(role)
	if err != nil {
		return TransportInfo{}, err
	}
	live, err := RenewableTransport(anchor, now)
	if err != nil {
		return TransportInfo{}, err
	}
	paired, err := transport.AuthenticatedPeerFromCertificate(anchor.Leaf)
	if err != nil {
		return TransportInfo{}, err
	}
	key, err := transport.ProductKeyFingerprint(anchor.Leaf)
	if err != nil {
		return TransportInfo{}, err
	}
	return TransportInfo{Fingerprint: paired.CertificateSHA256, Role: role, NotBefore: live.Leaf.NotBefore, NotAfter: live.Leaf.NotAfter,
		Expired: !now.Before(live.Leaf.NotAfter), AutomaticRenewal: true, KeyFingerprint: key}, nil
}
