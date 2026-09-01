package securefs

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrUnsafePath = errors.New("secure file path is unsafe")
	ErrHardLink   = errors.New("secure file has multiple hard links")
)

func validateLeaf(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name ||
		strings.ContainsAny(name, `/\\:`) || strings.ContainsRune(name, '\x00') {
		return ErrUnsafePath
	}
	if len(name) > 240 {
		return ErrUnsafePath
	}
	return nil
}

func temporaryName(prefix string) (string, error) {
	if prefix == "" || strings.ContainsAny(prefix, `/\\:`) || strings.ContainsRune(prefix, '\x00') {
		return "", ErrUnsafePath
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	name := prefix + hex.EncodeToString(random[:]) + ".tmp"
	if err := validateLeaf(name); err != nil {
		return "", err
	}
	return name, nil
}

func writeAndSync(file *os.File, data []byte) error {
	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}
