package productidentity

import (
	"errors"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
	"os"
)

// PrepareDiagnostics does not create or alter device keys or trust records.
func PrepareDiagnostics(localAppData string) (*securefs.Directory, error) {
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
	dir, err := root.CreateDirectory("diagnostics", 0700)
	if errors.Is(err, os.ErrExist) {
		if _, err = peercredentials.ValidateSecurityDirectory(paths.Diagnostics); err != nil {
			return nil, err
		}
		return securefs.OpenDirectory(paths.Diagnostics)
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
