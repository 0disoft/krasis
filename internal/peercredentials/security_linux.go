//go:build linux

package peercredentials

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/unix"
)

func ProtectPrivateKeyDirectory(path string) error {
	directory, err := ProtectSecurityDirectory(path)
	if err != nil {
		return err
	}
	return ProtectSecurityFile(filepath.Join(directory, "identity-key.pem"))
}

// Open the directory without following symlinks and apply policy to that same
// descriptor. A foreign owner is never repaired, even when running as root.
func ProtectSecurityDirectory(path string) (string, error) {
	return securityDirectory(path, true, nil)
}

func ValidateSecurityDirectory(path string) (string, error) {
	return ValidateSecurityDirectoryObserved(path, nil)
}

func ValidateSecurityDirectoryObserved(path string, observe func(string)) (string, error) {
	return securityDirectory(path, false, observe)
}

func securityDirectory(path string, protect bool, observe func(string)) (string, error) {
	if path == "" {
		return "", errors.New("security directory is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if observe != nil {
		observe("stat")
	}
	dir, err := securefs.OpenReadDirectory(absolute)
	if err != nil {
		return "", errors.Join(ErrInsecurePrivateKey, err)
	}
	defer dir.Close()
	if protect {
		err = ProtectSecurityDirectoryHandle(dir.Handle())
	} else {
		err = ValidateSecurityDirectoryHandle(dir.Handle())
	}
	if err != nil {
		return "", err
	}
	return absolute, nil
}

func ProtectSecurityDirectoryHandle(handle uintptr) error {
	if err := securityStat(handle, unix.S_IFDIR, 0, false); err != nil {
		return err
	}
	if err := unix.Fchmod(int(handle), 0o700); err != nil {
		return err
	}
	return ValidateSecurityDirectoryHandle(handle)
}

func ValidateSecurityDirectoryHandle(handle uintptr) error {
	return securityStat(handle, unix.S_IFDIR, 0o700, true)
}

func ProtectSecurityFile(path string) error        { return securityFile(path, true) }
func ValidateSecurityFile(path string) error       { return securityFile(path, false) }
func ValidatePrivateKeySecurity(path string) error { return ValidateSecurityFile(path) }

func securityFile(path string, protect bool) error {
	if path == "" {
		return errors.New("security file path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	dir, err := OpenProtectedDirectory(filepath.Dir(absolute), true)
	if err != nil {
		return err
	}
	defer dir.Close()
	file, err := dir.Open(filepath.Base(absolute))
	if err != nil {
		if errors.Is(err, securefs.ErrHardLink) || errors.Is(err, securefs.ErrUnsafePath) {
			return errors.Join(ErrInsecurePrivateKey, err)
		}
		return err
	}
	defer file.Close()
	if protect {
		return ProtectSecurityFileHandle(file)
	}
	return ValidateSecurityFileHandle(file)
}

func ProtectSecurityFileHandle(file *os.File) error {
	if file == nil {
		return errors.New("security file handle is required")
	}
	if err := securityStat(file.Fd(), unix.S_IFREG, 0, false); err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	return ValidateSecurityFileHandle(file)
}

func ValidateSecurityFileHandle(file *os.File) error {
	return ValidateSecurityFileHandleObserved(file, nil)
}

func ValidateSecurityFileHandleObserved(file *os.File, observe func(string)) error {
	if file == nil {
		return errors.New("security file handle is required")
	}
	if observe != nil {
		observe("info")
	}
	// Stat through os.File first so closed descriptors cannot be reused as a
	// different file between File.Fd and validation.
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if observe != nil {
		observe("acl_check")
	}
	if !info.Mode().IsRegular() {
		return ErrInsecurePrivateKey
	}
	return securityStat(file.Fd(), unix.S_IFREG, 0o600, true)
}

func securityStat(handle uintptr, kind, mode uint32, checkMode bool) error {
	if handle == ^uintptr(0) || uint64(handle) > uint64(^uint32(0)>>1) {
		return ErrInsecurePrivateKey
	}
	var stat unix.Stat_t
	if err := unix.Fstat(int(handle), &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != kind || stat.Uid != uint32(os.Geteuid()) ||
		kind == unix.S_IFREG && stat.Nlink != 1 || checkMode && stat.Mode&0o7777 != mode {
		return ErrInsecurePrivateKey
	}
	return nil
}
