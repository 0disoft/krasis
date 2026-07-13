//go:build !windows

package peercredentials

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func ProtectPrivateKeyDirectory(directory string) error {
	if directory == "" {
		return errors.New("credential directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("credential path must be a real directory")
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return fmt.Errorf("protect credential directory: %w", err)
	}
	keyPath := filepath.Join(absolute, "identity-key.pem")
	info, err = os.Lstat(keyPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("private key must be a regular file")
	}
	if err := os.Chmod(keyPath, 0o600); err != nil {
		return fmt.Errorf("protect private key: %w", err)
	}
	return ValidatePrivateKeySecurity(keyPath)
}

func ValidatePrivateKeySecurity(path string) error {
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
