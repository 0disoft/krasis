//go:build windows

package appinstall

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/securefs"
)

// Stage only the preflight-selected immutable image. No launch pointer,
// registration, receipt or previously existing image is changed here.
func stageInstallImage(path string, p Package, id string) error {
	root, err := os.OpenRoot(path)
	if err != nil {
		return installationFailure("stage_image", err)
	}
	err = p.Bundle.Stage(root, id)
	if errors.Is(err, os.ErrExist) {
		err = nil // Reuse only after matching the complete verified bundle below.
	}
	err = errors.Join(err, root.Close())
	if err != nil {
		return installationFailure("stage_image", err)
	}
	return verifyInstallImage(filepath.Join(path, id), p)
}

func verifyInstallImage(path string, p Package) error {
	guard, err := securefs.OpenReadDirectory(path)
	if err != nil {
		return installationFailure("verify_image", err)
	}
	err = errors.Join(p.Bundle.MatchDirectory(guard), guard.Close())
	return installationFailure("verify_image", err)
}
