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
	file, err := directory.Open(name)
	var document transportDocument
	if errors.Is(err, os.ErrNotExist) {
		document, err = createTransport(identity.Public(), role, now)
		if err == nil {
			err = saveTransport(directory, name, document)
		}
	} else if err == nil {
		defer file.Close()
		if err = peercredentials.ValidateSecurityFileHandle(file); err != nil {
			return tls.Certificate{}, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxStoreBytes+1))
		if readErr != nil {
			return tls.Certificate{}, readErr
		}
		if len(data) > maxStoreBytes {
			return tls.Certificate{}, errors.New("transport document exceeds size limit")
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
	if err != nil {
		return tls.Certificate{}, err
	}
	return validateTransport(document, identity.Public(), role, now)
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
	if document.SchemaVersion != "krasis.product-transport/v1" || document.IdentityFingerprint != identity.Fingerprint || document.Generation != identity.Generation {
		return tls.Certificate{}, ErrTransportRenewalRequired
	}
	leaf, err := x509.ParseCertificate(document.Certificate)
	if err != nil {
		return tls.Certificate{}, err
	}
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return tls.Certificate{}, ErrTransportRenewalRequired
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
	return directory.PromoteNoReplace(temporary, name)
}
