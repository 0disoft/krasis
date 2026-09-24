package truststore

import (
	"bytes"
	"crypto/x509"
	"errors"
	"io"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/transport"
)

const maxPeerCertificateBytes = 16 << 10

var ErrPeerCertificateUnavailable = errors.New("paired peer certificate is unavailable")

// ReconfirmWithCertificate is a read-only persistence check after mutual pairing
// confirmation, not a grant or repair operation. Existing policy and bytes must
// match exactly; revoked peers and missing certificates are never restored.
func (store *Store) ReconfirmWithCertificate(peer transport.AuthenticatedPeerContext, der []byte, alias string, allowInboundControl bool, now time.Time) (PeerRecord, error) {
	if store == nil {
		return PeerRecord{}, errors.New("trust store is not initialized")
	}
	der = bytes.Clone(der)
	if _, err := validatePeerCertificate(peer, der, now); err != nil {
		return PeerRecord{}, err
	}
	document, directory, release, err := store.openReadLocked()
	if err != nil {
		return PeerRecord{}, err
	}
	defer release()
	for _, record := range document.Peers {
		if record.Fingerprint != peer.CertificateSHA256 {
			continue
		}
		if record.RevokedAt != nil {
			return PeerRecord{}, ErrPeerRevoked
		}
		if record.Role != peer.Role || record.Alias != alias || record.AllowInboundControl != allowInboundControl {
			return PeerRecord{}, errors.New("existing peer policy does not match confirmation")
		}
		stored, err := readPeerCertificate(directory, peer.CertificateSHA256)
		if err != nil {
			return PeerRecord{}, err
		}
		if !bytes.Equal(stored, der) {
			return PeerRecord{}, errors.New("stored peer certificate mismatch")
		}
		return record, nil
	}
	return PeerRecord{}, ErrPeerNotFound
}

// ImportWithCertificate is a persistence primitive, not a pairing bypass.
// Callers must first verify mutual confirmation for this exact peer context.
func (store *Store) ImportWithCertificate(peer transport.AuthenticatedPeerContext, der []byte, alias string, allowInboundControl bool, now time.Time) (PeerRecord, error) {
	der = bytes.Clone(der)
	if _, err := validatePeerCertificate(peer, der, now); err != nil {
		return PeerRecord{}, err
	}
	return store.importPeer(peer, alias, allowInboundControl, now, func(directory *securefs.Directory) error {
		return persistPeerCertificate(directory, peer.CertificateSHA256, der)
	})
}

// ReplaceWithCertificate requires a mutually confirmed replacement peer and an
// explicitly selected old fingerprint. It does not transfer connection settings.
// The certificate is prepared under the store lock before the single trust commit.
func (store *Store) ReplaceWithCertificate(fingerprint string, replacement transport.AuthenticatedPeerContext, der []byte, alias string, allowInboundControl bool, now time.Time) (PeerRecord, PeerRecord, error) {
	der = bytes.Clone(der)
	if _, err := validatePeerCertificate(replacement, der, now); err != nil {
		return PeerRecord{}, PeerRecord{}, err
	}
	return store.replacePeer(fingerprint, replacement, alias, allowInboundControl, now, func(directory *securefs.Directory) error {
		return persistPeerCertificate(directory, replacement.CertificateSHA256, der)
	})
}

// PeerCertificate checks current trust and certificate validity under the same
// store lock. A cached certificate must not replace this check on reconnect.
func (store *Store) PeerCertificate(peer transport.AuthenticatedPeerContext, now time.Time) (*x509.Certificate, error) {
	if store == nil {
		return nil, errors.New("trust store is not initialized")
	}
	if err := peer.Validate(); err != nil {
		return nil, err
	}
	document, directory, release, err := store.openReadLocked()
	if err != nil {
		return nil, err
	}
	defer release()
	for _, record := range document.Peers {
		if record.Fingerprint != peer.CertificateSHA256 {
			continue
		}
		if record.RevokedAt != nil {
			return nil, ErrPeerRevoked
		}
		if record.Role != peer.Role {
			return nil, errors.New("trusted peer role mismatch")
		}
		der, err := readPeerCertificate(directory, peer.CertificateSHA256)
		if err != nil {
			return nil, err
		}
		return validatePeerCertificate(peer, der, now)
	}
	return nil, ErrPeerNotFound
}

func validatePeerCertificate(peer transport.AuthenticatedPeerContext, der []byte, now time.Time) (*x509.Certificate, error) {
	if err := peer.Validate(); err != nil {
		return nil, err
	}
	if now.IsZero() || len(der) == 0 || len(der) > maxPeerCertificateBytes {
		return nil, errors.New("peer certificate and validation time are required")
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	actual, err := transport.AuthenticatedPeerFromCertificate(certificate)
	if err != nil || actual != peer {
		return nil, errors.New("certificate does not match confirmed peer")
	}
	if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
		return nil, errors.New("peer certificate is outside its validity window")
	}
	if certificate.IsCA || certificate.KeyUsage != x509.KeyUsageDigitalSignature {
		return nil, errors.New("peer certificate must be an end-entity signing certificate")
	}
	// Product transport credentials are self-signed leaves, never imported CAs.
	if err := certificate.CheckSignature(certificate.SignatureAlgorithm, certificate.RawTBSCertificate, certificate.Signature); err != nil {
		return nil, err
	}
	return certificate, nil
}

func peerCertificateFilename(fingerprint string) string { return "certificate-" + fingerprint + ".der" }

func readPeerCertificate(directory *securefs.Directory, fingerprint string) ([]byte, error) {
	file, err := directory.Open(peerCertificateFilename(fingerprint))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrPeerCertificateUnavailable
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := peercredentials.ValidateSecurityFileHandle(file); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPeerCertificateBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxPeerCertificateBytes {
		return nil, errors.New("peer certificate exceeds size limit")
	}
	return data, nil
}

func persistPeerCertificate(directory *securefs.Directory, fingerprint string, der []byte) error {
	// A previous failed trust commit may leave this public, unauthorized blob.
	// Reuse it only if its protected bytes are identical; never overwrite damage.
	existing, err := readPeerCertificate(directory, fingerprint)
	if err == nil {
		if !bytes.Equal(existing, der) {
			return errors.New("stored peer certificate mismatch")
		}
		return nil
	}
	if !errors.Is(err, ErrPeerCertificateUnavailable) {
		return err
	}
	file, name, err := directory.CreateTemporary(".certificate-", 0o600)
	if err != nil {
		return err
	}
	defer directory.Remove(name)
	defer file.Close()
	if err := peercredentials.ProtectSecurityFileHandle(file); err != nil {
		return err
	}
	if _, err := file.Write(der); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return directory.PromoteNoReplace(name, peerCertificateFilename(fingerprint))
}
