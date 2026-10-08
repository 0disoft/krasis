//go:build windows

package appinstall

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

var errDamagedInstallFolder = errors.New("installed folder differs from verified bundle")

type installFolderOps struct {
	match func(string) error
	newID func() (string, error)
}

// Probe the existing directory without creating or modifying any image. A
// missing file inside an existing image is damage; an unreadable owner is not.
func prepareInstallFolder(path string, p Package, next Receipt) (Receipt, error) {
	return selectInstallFolder(next, installFolderOps{
		match: func(id string) error {
			guard, err := securefs.OpenReadDirectory(filepath.Join(path, id))
			if err != nil {
				return err
			}
			err = p.Bundle.MatchDirectory(guard)
			closeErr := guard.Close()
			if closeErr != nil {
				return errors.Join(err, closeErr)
			}
			if errors.Is(err, updatebundle.ErrInvalid) || errors.Is(err, os.ErrNotExist) {
				return errDamagedInstallFolder
			}
			return err
		},
		newID: func() (string, error) {
			var id [16]byte
			if _, err := rand.Read(id[:]); err != nil {
				return "", err
			}
			return hex.EncodeToString(id[:]), nil
		},
	})
}

func selectInstallFolder(next Receipt, ops installFolderOps) (Receipt, error) {
	err := ops.match(next.Folders[0])
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return next, nil
	}
	if !errors.Is(err, errDamagedInstallFolder) {
		return Receipt{}, err
	}
	// Reserve the additional repair entry before creating a directory or
	// asking the running product to stop. Preserve every existing entry.
	if len(next.Folders) >= 64 {
		return Receipt{}, ErrInvalid
	}
	id, err := ops.newID()
	if err != nil {
		return Receipt{}, err
	}
	if len(id) != 32 || slices.Contains(next.Folders, id) {
		return Receipt{}, ErrInvalid
	}
	bytes, err := hex.DecodeString(id)
	if err != nil || hex.EncodeToString(bytes) != id {
		return Receipt{}, ErrInvalid
	}
	// A random-ID collision must not adopt an existing image or foreign data.
	if err := ops.match(id); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = ErrInvalid
		}
		return Receipt{}, err
	}
	next.Folders = append([]string{id}, next.Folders...)
	return next, nil
}
