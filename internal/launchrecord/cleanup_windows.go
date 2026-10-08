//go:build windows

package launchrecord

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/0disoft/krasis/internal/securefs"
)

// ValidateRecordRemoval checks both records without inspecting their executable
// bytes: installation removal may include a damaged or already removed image.
func ValidateRecordRemoval(base string, directories []string) error {
	_, err := recordNamesInDirectories(base, directories)
	return err
}

func recordNamesInDirectories(base string, directories []string) ([]string, error) {
	for _, path := range directories {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, ErrInvalid
		}
	}
	var names []string
	for _, name := range []string{"current.json", "previous.json"} {
		r, err := loadNamed(base, name)
		if errors.Is(err, ErrMissing) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, dir := range directories {
			if strings.EqualFold(filepath.Dir(r.Executable), dir) {
				names = append(names, name)
				break
			}
		}
	}
	return names, nil
}

// RemoveRecordsInDirectories removes only records for the receipt-owned image
// directories. Re-read under Register's lock so a newer portable launch cannot
// replace a preflight record and then have its receipt deleted by uninstall.
func RemoveRecordsInDirectories(base string, directories []string) error {
	path, err := Directory(base)
	if err != nil {
		return err
	}
	dir, err := securefs.OpenDirectory(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	unlock, err := lockRegistration(dir)
	if err != nil {
		return err
	}
	defer unlock()
	names, err := recordNamesInDirectories(base, directories)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := dir.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
