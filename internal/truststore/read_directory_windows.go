//go:build windows

package truststore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

func (store *Store) openReadDirectory(trace *ReadTrace) (*securefs.Directory, error) {
	trace.enterStep("prelock_parent_acl")
	var observe func(string)
	if trace != nil {
		observe = func(step string) { trace.enterDetail("prelock_parent_" + step) }
	}
	if _, err := peercredentials.ValidateSecurityDirectoryObserved(filepath.Dir(store.directory), observe); err != nil {
		return nil, fmt.Errorf("validate trust store parent security: %w", err)
	}
	trace.enterStep("prelock_directory_open")
	// Validate the same no-reparse handle used to open the lock, document and
	// certificate. No cached security verdict or writable directory is needed.
	directory, err := securefs.OpenReadDirectory(store.directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrStoreNotFound
	}
	if err != nil {
		return nil, err
	}
	trace.enterStep("prelock_directory_acl")
	if err := peercredentials.ValidateSecurityDirectoryHandle(directory.Handle()); err != nil {
		_ = directory.Close()
		return nil, err
	}
	return directory, nil
}
