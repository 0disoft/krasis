package productidentity

import (
	"errors"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

// Steady-state TLS, input-start and service-policy readers must not acquire an
// exclusive identity lock against each other. Key replacement still excludes
// every reader. Missing lock files are errors, never recreated by this path.
func (store *Store) openTransportReadLocked() (*securefs.Directory, func(), error) {
	if store == nil {
		return nil, nil, ErrIdentityNotFound
	}
	if err := store.prepareDirectory(false); err != nil {
		return nil, nil, err
	}
	dir, err := securefs.OpenReadDirectory(store.directory)
	if err != nil {
		return nil, nil, err
	}
	lock, err := dir.Open(lockFile)
	if err != nil {
		return nil, nil, errors.Join(err, dir.Close())
	}
	closeHandles := func() { _ = lock.Close(); _ = dir.Close() }
	if err := peercredentials.ValidateSecurityFileHandle(lock); err != nil {
		closeHandles()
		return nil, nil, err
	}
	unlock, err := acquireSharedFileLock(lock)
	if err != nil {
		closeHandles()
		return nil, nil, err
	}
	return dir, func() { _ = unlock(); closeHandles() }, nil
}
