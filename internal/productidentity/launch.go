package productidentity

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

// PrepareLaunch creates only the per-user recovery directory, never device keys.
func PrepareLaunch(localAppData string) (*securefs.Directory, error) {
	paths, err := ResolveWindowsPaths(localAppData)
	if err != nil {
		return nil, err
	}
	if err = ensureProductRoot(paths); err != nil {
		return nil, err
	}
	root, err := securefs.OpenDirectory(paths.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	path := filepath.Join(paths.Root, "launch")
	dir, err := root.CreateDirectory("launch", 0700)
	if errors.Is(err, os.ErrExist) {
		if _, err = peercredentials.ValidateSecurityDirectory(path); err != nil {
			return nil, err
		}
		return securefs.OpenDirectory(path)
	}
	if err != nil {
		return nil, err
	}
	if err = peercredentials.ProtectSecurityDirectoryHandle(dir.Handle()); err != nil {
		dir.Close()
		return nil, err
	}
	return dir, nil
}
