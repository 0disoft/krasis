package productidentity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/transport"
)

// TransportLifetime bounds this initial credential's validity. Renewal is a
// separate, explicit pairing operation; OpenTransport never extends this window.
const TransportLifetime = 30 * 24 * time.Hour

// TransportServerName is checked in addition to the paired certificate pin.
// It is not a discovery address or a substitute for authenticating the peer.
const TransportServerName = "krasis.invalid"

var ErrTransportRenewalRequired = errors.New("transport credential requires explicit renewal and pairing")

var ErrTransportFingerprintChanged = errors.New("transport fingerprint changed")

// TransportInfo contains only public metadata, never certificate or key bytes.
type TransportInfo struct {
	Fingerprint string                          `json:"fingerprint"`
	Role        transport.AuthenticatedPeerRole `json:"role"`
	NotBefore   time.Time                       `json:"notBefore"`
	NotAfter    time.Time                       `json:"notAfter"`
	Expired     bool                            `json:"expired"`
}

func (store *Store) InspectTransport(role transport.AuthenticatedPeerRole, now time.Time) (TransportInfo, error) {
	return store.maintainTransport(role, "", now)
}

// RenewTransport replaces exactly the expected certificate under the identity
// lock. It neither changes device identity nor grants trust on another device.
func (store *Store) RenewTransport(role transport.AuthenticatedPeerRole, expectedFingerprint string, now time.Time) (TransportInfo, error) {
	if expectedFingerprint == "" {
		return TransportInfo{}, ErrTransportFingerprintChanged
	}
	return store.maintainTransport(role, expectedFingerprint, now)
}

func (store *Store) maintainTransport(role transport.AuthenticatedPeerRole, expected string, now time.Time) (TransportInfo, error) {
	if store == nil || now.IsZero() || (role != transport.AuthenticatedPeerListener && role != transport.AuthenticatedPeerConnector) {
		return TransportInfo{}, errors.New("transport store, role and time are required")
	}
	directory, release, err := store.openLocked(false)
	if err != nil {
		return TransportInfo{}, err
	}
	defer release()
	identity, err := loadFromDirectory(directory)
	if err != nil {
		return TransportInfo{}, err
	}
	name := "transport-" + string(role) + ".json"
	document, err := readTransport(directory, name)
	if err != nil {
		return TransportInfo{}, err
	}
	certificate, err := validateTransportMaterial(document, identity.Public(), role)
	if err != nil {
		return TransportInfo{}, err
	}
	peer, err := transport.AuthenticatedPeerFromCertificate(certificate.Leaf)
	if err != nil {
		return TransportInfo{}, err
	}
	if expected != "" {
		if peer.CertificateSHA256 != expected {
			return TransportInfo{}, ErrTransportFingerprintChanged
		}
		next, err := createTransport(identity.Public(), role, now)
		if err != nil {
			return TransportInfo{}, err
		}
		certificate, err = validateTransport(next, identity.Public(), role, now)
		if err != nil {
			return TransportInfo{}, err
		}
		peer, err = transport.AuthenticatedPeerFromCertificate(certificate.Leaf)
		if err != nil {
			return TransportInfo{}, err
		}
		if err := writeTransport(directory, name, next, true); err != nil {
			return TransportInfo{}, err
		}
	}
	return TransportInfo{Fingerprint: peer.CertificateSHA256, Role: role, NotBefore: certificate.Leaf.NotBefore, NotAfter: certificate.Leaf.NotAfter, Expired: !now.Before(certificate.Leaf.NotAfter)}, nil
}

type transportDocument struct {
	SchemaVersion       string `json:"schemaVersion"`
	IdentityFingerprint string `json:"identityFingerprint"`
	Generation          uint64 `json:"generation"`
	Certificate         []byte `json:"certificate"`
	PrivateKey          []byte `json:"privateKey"`
}

// OpenTransport keeps a separate key for each TLS role. Existing credentials are
// never silently replaced: paired peers authorize their exact certificate hash.
func (store *Store) OpenTransport(role transport.AuthenticatedPeerRole, now time.Time) (tls.Certificate, error) {
	if now.IsZero() || (role != transport.AuthenticatedPeerListener && role != transport.AuthenticatedPeerConnector) {
		return tls.Certificate{}, errors.New("transport role and time are required")
	}
	directory, release, err := store.openLocked(false)
	if err != nil {
		return tls.Certificate{}, err
	}
	defer release()
	identity, err := loadFromDirectory(directory)
	if err != nil {
		return tls.Certificate{}, err
	}
	name := "transport-" + string(role) + ".json"
	document, err := readTransport(directory, name)
	if errors.Is(err, os.ErrNotExist) {
		document, err = createTransport(identity.Public(), role, now)
		if err == nil {
			err = saveTransport(directory, name, document)
		}
	}
	if err != nil {
		return tls.Certificate{}, err
	}
	return validateTransport(document, identity.Public(), role, now)
}

func readTransport(directory *securefs.Directory, name string) (transportDocument, error) {
	return readTransportVerified(directory, name, peercredentials.ValidateSecurityFileHandle)
}

func readTransportVerified(directory *securefs.Directory, name string, validate func(*os.File) error) (transportDocument, error) {
	var document transportDocument
	file, err := directory.Open(name)
	if err == nil {
		defer file.Close()
		if err = validate(file); err != nil {
			return document, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxStoreBytes+1))
		if readErr != nil {
			return document, readErr
		}
		if len(data) > maxStoreBytes {
			return document, errors.New("transport document exceeds size limit")
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&document)
		if err == nil {
			var extra any
			if !errors.Is(decoder.Decode(&extra), io.EOF) {
				err = errors.New("trailing transport document")
			}
		}
	}
	return document, err
}

func createTransport(identity PublicIdentity, role transport.AuthenticatedPeerRole, now time.Time) (transportDocument, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return transportDocument{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return transportDocument{}, err
	}
	serial.Add(serial, big.NewInt(1))
	usage := x509.ExtKeyUsageClientAuth
	if role == transport.AuthenticatedPeerListener {
		usage = x509.ExtKeyUsageServerAuth
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "Krasis product transport"},
		NotBefore: now.UTC().Add(-time.Minute), NotAfter: now.UTC().Add(TransportLifetime),
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{usage},
		DNSNames:    []string{TransportServerName},
	}
	certificate, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		return transportDocument{}, err
	}
	key, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		return transportDocument{}, err
	}
	return transportDocument{SchemaVersion: "krasis.product-transport/v1", IdentityFingerprint: identity.Fingerprint,
		Generation: identity.Generation, Certificate: certificate, PrivateKey: key}, nil
}

func validateTransport(document transportDocument, identity PublicIdentity, role transport.AuthenticatedPeerRole, now time.Time) (tls.Certificate, error) {
	certificate, err := validateTransportMaterial(document, identity, role)
	if err != nil {
		return tls.Certificate{}, err
	}
	if now.Before(certificate.Leaf.NotBefore) || !now.Before(certificate.Leaf.NotAfter) {
		return tls.Certificate{}, ErrTransportRenewalRequired
	}
	return certificate, nil
}

func validateTransportMaterial(document transportDocument, identity PublicIdentity, role transport.AuthenticatedPeerRole) (tls.Certificate, error) {
	if document.SchemaVersion != "krasis.product-transport/v1" || document.IdentityFingerprint != identity.Fingerprint || document.Generation != identity.Generation {
		return tls.Certificate{}, ErrTransportRenewalRequired
	}
	leaf, err := x509.ParseCertificate(document.Certificate)
	if err != nil {
		return tls.Certificate{}, err
	}
	if leaf.IsCA || leaf.KeyUsage != x509.KeyUsageDigitalSignature ||
		leaf.NotAfter.Sub(leaf.NotBefore) > TransportLifetime+time.Minute ||
		len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != TransportServerName {
		return tls.Certificate{}, errors.New("transport certificate policy mismatch")
	}
	peer, err := transport.AuthenticatedPeerFromCertificate(leaf)
	if err != nil || peer.Role != role {
		return tls.Certificate{}, errors.New("transport certificate role mismatch")
	}
	if err := leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature); err != nil {
		return tls.Certificate{}, err
	}
	key, err := x509.ParsePKCS8PrivateKey(document.PrivateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	private, ok := key.(ed25519.PrivateKey)
	public, publicOK := leaf.PublicKey.(ed25519.PublicKey)
	if !ok || !publicOK || !bytes.Equal(private.Public().(ed25519.PublicKey), public) {
		return tls.Certificate{}, errors.New("transport certificate and key mismatch")
	}
	return tls.Certificate{Certificate: [][]byte{document.Certificate}, PrivateKey: private, Leaf: leaf}, nil
}

func saveTransport(directory *securefs.Directory, name string, document transportDocument) error {
	return writeTransport(directory, name, document, false)
}

func writeTransport(directory *securefs.Directory, name string, document transportDocument, replace bool) error {
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	file, temporary, err := directory.CreateTemporary(".transport-", 0o600)
	if err != nil {
		return err
	}
	defer directory.Remove(temporary)
	defer file.Close()
	if err := peercredentials.ProtectSecurityFileHandle(file); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if replace {
		return directory.Replace(temporary, name)
	}
	return directory.PromoteNoReplace(temporary, name)
}
