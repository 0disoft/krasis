//go:build windows

package productidentity

import (
	"crypto/tls"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/transport"
)

// ReadOwnerTransport reads existing credentials under their original profile
// ACL and identity lock. It cannot create, rotate or repair any credential.
func ReadOwnerTransport(localAppData, owner string, role transport.AuthenticatedPeerRole, now time.Time) (PublicIdentity, tls.Certificate, error) {
	return readOwnerTransport(localAppData, owner, role, now, false)
}

// ReadOwnerTransportAnchor allows only in-memory reissuance of an existing
// verified role key; it still never writes to the selected user's store.
func ReadOwnerTransportAnchor(localAppData, owner string, role transport.AuthenticatedPeerRole, now time.Time) (PublicIdentity, tls.Certificate, error) {
	return readOwnerTransport(localAppData, owner, role, now, true)
}

func readOwnerTransport(localAppData, owner string, role transport.AuthenticatedPeerRole, now time.Time, keyAnchor bool) (PublicIdentity, tls.Certificate, error) {
	if now.IsZero() || role != transport.AuthenticatedPeerListener && role != transport.AuthenticatedPeerConnector {
		return PublicIdentity{}, tls.Certificate{}, ErrTransportRenewalRequired
	}
	paths, err := ResolveWindowsPaths(localAppData)
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	scope, err := peercredentials.ReadScopeForOwner(owner)
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	root, err := scope.OpenDirectory(paths.Root)
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	defer root.Close()
	dir, err := scope.OpenDirectory(filepath.Join(paths.Root, "identity"))
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	defer dir.Close()
	lock, err := dir.Open(lockFile)
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	defer lock.Close()
	if err := scope.ValidateFile(lock); err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	acquire := acquireFileLock
	if keyAnchor {
		acquire = acquireSharedFileLock
	}
	unlock, err := acquire(lock)
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	defer unlock()
	identity, err := loadFromDirectoryVerified(dir, scope.ValidateFile)
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	doc, err := readTransportVerified(dir, "transport-"+string(role)+".json", scope.ValidateFile)
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	cert, err := validateTransport(doc, identity.Public(), role, now)
	if keyAnchor {
		cert, err = validateTransportMaterial(doc, identity.Public(), role)
	}
	if err != nil {
		return PublicIdentity{}, tls.Certificate{}, err
	}
	return identity.Public(), cert, nil
}
