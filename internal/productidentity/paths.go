package productidentity

import (
	"errors"
	"path/filepath"
	"strings"
)

type Paths struct {
	Root        string
	Identity    string
	Trust       string
	Diagnostics string
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
