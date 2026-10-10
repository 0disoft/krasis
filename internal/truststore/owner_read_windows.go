//go:build windows

package truststore

import (
	"crypto/x509"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/transport"
)

// ReadOwnerConnection preserves the original profile ACL and shared trust lock.
// It cannot import a peer, grant permission, update an address or create files.
func ReadOwnerConnection(localAppData, owner, alias, fingerprint string, now time.Time) (PeerRecord, ConnectionProfile, *x509.Certificate, error) {
	return readOwnerConnection(localAppData, owner, alias, fingerprint, now, false)
}

func ReadOwnerKeyConnection(localAppData, owner, alias, fingerprint string, now time.Time) (PeerRecord, ConnectionProfile, *x509.Certificate, error) {
	return readOwnerConnection(localAppData, owner, alias, fingerprint, now, true)
}

func readOwnerConnection(localAppData, owner, alias, fingerprint string, now time.Time, keyAnchor bool) (PeerRecord, ConnectionProfile, *x509.Certificate, error) {
	deny := func(err error) (PeerRecord, ConnectionProfile, *x509.Certificate, error) {
		return PeerRecord{}, ConnectionProfile{}, nil, err
	}
	if now.IsZero() || validateFingerprint(fingerprint) != nil {
		return deny(ErrPeerNotFound)
	}
	paths, err := productidentity.ResolveWindowsPaths(localAppData)
	if err != nil {
		return deny(err)
	}
	scope, err := peercredentials.ReadScopeForOwner(owner)
	if err != nil {
		return deny(err)
	}
	root, err := scope.OpenDirectory(paths.Root)
	if err != nil {
		return deny(err)
	}
	defer root.Close()
	dir, err := scope.OpenDirectory(filepath.Join(paths.Root, "trust"))
	if err != nil {
		return deny(err)
	}
	defer dir.Close()
	lock, err := dir.Open(lockFilename)
	if err != nil {
		return deny(err)
	}
	defer lock.Close()
	if err := scope.ValidateFile(lock); err != nil {
		return deny(err)
	}
	unlock, err := acquireFileLock(lock, true)
	if err != nil {
		return deny(err)
	}
	defer unlock()
	doc, err := readVerified(dir, nil, scope.ValidateFile)
	if err != nil {
		return deny(err)
	}
	peer, err := resolveConnectionPeer(doc, alias)
	if err != nil || peer.Fingerprint != fingerprint {
		return deny(ErrPeerNotFound)
	}
	connection, err := readConnectionVerified(dir, fingerprint, scope.ValidateFile)
	if err != nil {
		return deny(err)
	}
	der, err := readPeerCertificateVerified(dir, fingerprint, scope.ValidateFile)
	if err != nil {
		return deny(err)
	}
	certificate, err := validatePeerCertificate(transport.AuthenticatedPeerContext{CertificateSHA256: fingerprint, Role: peer.Role}, der, now)
	if keyAnchor {
		certificate, err = validatePeerKeyAnchor(transport.AuthenticatedPeerContext{CertificateSHA256: fingerprint, Role: peer.Role}, der, now)
	}
	if err != nil {
		return deny(err)
	}
	return peer, connection, certificate, nil
}
