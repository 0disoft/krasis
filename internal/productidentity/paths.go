package productidentity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/securefs"
)

type Paths struct {
	Root        string
	Identity    string
	Trust       string
	Diagnostics string
}

func BootstrapWindows(localAppData string, now time.Time) (Paths, *Identity, bool, error) {
	paths, err := ResolveWindowsPaths(localAppData)
	if err != nil {
		return Paths{}, nil, false, err
	}
	if err := ensureProductRoot(paths); err != nil {
		return Paths{}, nil, false, err
	}
	store, err := New(paths.Identity)
	if err != nil {
		return Paths{}, nil, false, err
	}
	identity, created, err := store.OpenOrCreate(now)
	if err != nil {
		return Paths{}, nil, false, err
	}
	return paths, identity, created, nil
}

func ensureProductRoot(paths Paths) error {
	info, err := os.Lstat(paths.Root)
	if err == nil {
		if !info.IsDir() {
			return errors.New("Krasis product data root must be a directory")
		}
		if _, err := peercredentials.ValidateSecurityDirectory(paths.Root); err != nil {
			return fmt.Errorf("validate existing Krasis product data root: %w", err)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Krasis product data root: %w", err)
	}
	parent, err := securefs.OpenDirectory(filepath.Dir(paths.Root))
	if err != nil {
		return fmt.Errorf("open LOCALAPPDATA directory: %w", err)
	}
	defer parent.Close()
	root, err := parent.CreateDirectory(filepath.Base(paths.Root), 0o700)
	if err != nil {
		return fmt.Errorf("create Krasis product data root: %w", err)
	}
	defer root.Close()
	if err := peercredentials.ProtectSecurityDirectoryHandle(root.Handle()); err != nil {
		_ = root.RemoveSelf()
		return fmt.Errorf("protect Krasis product data root: %w", err)
	}
	return nil
}

func ResolveWindowsPaths(localAppData string) (Paths, error) {
	if localAppData == "" || strings.TrimSpace(localAppData) != localAppData {
		return Paths{}, errors.New("LOCALAPPDATA is required without surrounding whitespace")
	}
	absolute, err := filepath.Abs(localAppData)
	if err != nil {
		return Paths{}, err
	}
	root := filepath.Join(absolute, "Krasis")
	return Paths{
		Root:        root,
		Identity:    filepath.Join(root, "identity"),
		Trust:       filepath.Join(root, "trust"),
		Diagnostics: filepath.Join(root, "diagnostics"),
	}, nil
}
