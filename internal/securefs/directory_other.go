//go:build !windows

package securefs

import (
	"errors"
	"os"
	"path/filepath"
)

type Directory struct {
	path string
}

func OpenDirectory(path string) (*Directory, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafePath
	}
	return &Directory{path: absolute}, nil
}

func (directory *Directory) Close() error { return nil }

func (directory *Directory) Handle() uintptr { return 0 }

func (directory *Directory) Open(name string) (*os.File, error) {
	return directory.openExisting(name, os.O_RDONLY)
}

func (directory *Directory) OpenReadWrite(name string) (*os.File, error) {
	return directory.openExisting(name, os.O_RDWR)
}

func (directory *Directory) CreateExclusive(name string, mode os.FileMode) (*os.File, error) {
	if directory == nil {
		return nil, errors.New("secure directory is closed")
	}
	if err := validateLeaf(name); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(directory.path, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
}

func (directory *Directory) CreateTemporary(prefix string, mode os.FileMode) (*os.File, string, error) {
	for attempt := 0; attempt < 16; attempt++ {
		name, err := temporaryName(prefix)
		if err != nil {
			return nil, "", err
		}
		file, err := directory.CreateExclusive(name, mode)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", errors.New("secure temporary name collision limit reached")
}

func (directory *Directory) openExisting(name string, flag int) (*os.File, error) {
	if directory == nil {
		return nil, errors.New("secure directory is closed")
	}
	if err := validateLeaf(name); err != nil {
		return nil, err
	}
	path := filepath.Join(directory.path, name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnsafePath
	}
	return os.OpenFile(path, flag, 0)
}

func (directory *Directory) Replace(source, destination string) error {
	return directory.rename(source, destination, true)
}

func (directory *Directory) PromoteNoReplace(source, destination string) error {
	if _, err := os.Lstat(filepath.Join(directory.path, destination)); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return directory.rename(source, destination, false)
}

func (directory *Directory) Remove(name string) error {
	if directory == nil {
		return errors.New("secure directory is closed")
	}
	if err := validateLeaf(name); err != nil {
		return err
	}
	return os.Remove(filepath.Join(directory.path, name))
}

func (directory *Directory) rename(source, destination string, _ bool) error {
	if directory == nil {
		return errors.New("secure directory is closed")
	}
	if err := validateLeaf(source); err != nil {
		return err
	}
	if err := validateLeaf(destination); err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Join(directory.path, source))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafePath
	}
	return os.Rename(filepath.Join(directory.path, source), filepath.Join(directory.path, destination))
}
