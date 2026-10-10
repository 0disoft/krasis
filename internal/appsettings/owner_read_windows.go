//go:build windows

package appsettings

import (
	"io"
	"path/filepath"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

// ReadOwnerStartup reads both live policy files without defaults, writes or ACL
// repair. Missing/damaged policy cannot enable the privileged boot connection.
func ReadOwnerStartup(base, owner string) (Settings, Resume, error) {
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return Settings{}, Resume{}, err
	}
	scope, err := peercredentials.ReadScopeForOwner(owner)
	if err != nil {
		return Settings{}, Resume{}, err
	}
	root, err := scope.OpenDirectory(paths.Root)
	if err != nil {
		return Settings{}, Resume{}, err
	}
	defer root.Close()
	dir, err := scope.OpenDirectory(filepath.Join(paths.Root, "launch"))
	if err != nil {
		return Settings{}, Resume{}, err
	}
	defer dir.Close()
	load := func(name string, value any) error {
		file, err := dir.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := scope.ValidateFile(file); err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(file, 4097))
		if err != nil || len(data) > 4096 {
			return errInvalidDocument
		}
		return decodeSettings(data, value)
	}
	settings, err := loadSettings(func(v any) error { return load("settings.json", v) }, false)
	if err != nil {
		return Settings{}, Resume{}, err
	}
	resume, err := loadResume(func(v any) error { return load("resume.json", v) }, false)
	if err != nil {
		return Settings{}, Resume{}, err
	}
	return settings, resume, nil
}
