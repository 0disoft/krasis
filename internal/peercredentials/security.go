package peercredentials

import (
	"errors"

	"github.com/0disoft/krasis/internal/securefs"
)

var ErrInsecurePrivateKey = errors.New("security file policy is not satisfied")

// OpenProtectedDirectory validates the exact handle later used for child I/O.
func OpenProtectedDirectory(path string, readOnly bool) (*securefs.Directory, error) {
	open := securefs.OpenDirectory
	if readOnly {
		open = securefs.OpenReadDirectory
	}
	directory, err := open(path)
	if err != nil {
		return nil, err
	}
	if err := ValidateSecurityDirectoryHandle(directory.Handle()); err != nil {
		return nil, errors.Join(err, directory.Close())
	}
	return directory, nil
}
