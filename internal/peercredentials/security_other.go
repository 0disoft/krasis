//go:build !windows

package peercredentials

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func ProtectPrivateKeyDirectory(directory string) error {
	absolute, err := ProtectSecurityDirectory(directory)
	if err != nil {
		return err
	}
	return ProtectSecurityFile(filepath.Join(absolute, "identity-key.pem"))
}

func ProtectSecurityDirectory(directory string) (string, error) {
	if directory == "" {
		return "", errors.New("security directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("security path must be a real directory")
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return "", fmt.Errorf("protect security directory: %w", err)
	}
	return absolute, nil
}

func ValidateSecurityDirectory(directory string) (string, error) {
	if directory == "" {
		return "", errors.New("security directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return "", ErrInsecurePrivateKey
	}
	return absolute, nil
}

func ProtectSecurityFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("security file must be a regular file")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect security file: %w", err)
	}
	return ValidateSecurityFile(path)
}

func ValidatePrivateKeySecurity(path string) error {
	return ValidateSecurityFile(path)
}

func ValidateSecurityFile(path string) error {
	if path == "" {
		return errors.New("private key path is required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		return ErrInsecurePrivateKey
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return err
	}
	if !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm() != 0o700 {
		return ErrInsecurePrivateKey
	}
	return nil
}
