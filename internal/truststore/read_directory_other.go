//go:build !windows

package truststore

import "github.com/0disoft/krasis/internal/securefs"

func (store *Store) openReadDirectory(trace *ReadTrace) (*securefs.Directory, error) {
	if err := store.prepareDirectoryTraced(false, trace); err != nil {
		return nil, err
	}
	trace.enterStep("prelock_directory_open")
	return securefs.OpenReadDirectory(store.directory)
}
