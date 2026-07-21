package launcher

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const maxChecksumManifestBytes = 16 << 10

var (
	checksumLine = regexp.MustCompile(`^([0-9a-f]{64})  ([^/\\]+)$`)
	kitFiles     = []string{
		"krasis-lab.exe",
		"krasis-launcher.exe",
		"collect-probe.ps1",
		"check-phase0.ps1",
		"verify-kit.ps1",
		"README.md",
		"BUILD.json",
	}
)

func VerifyKit(base string) error {
	manifestPath := filepath.Join(base, "SHA256SUMS.txt")
	info, err := os.Lstat(manifestPath)
	if err != nil {
		return fmt.Errorf("checksum manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxChecksumManifestBytes {
		return errors.New("checksum manifest must be a regular bounded file")
	}
	manifest, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer manifest.Close()

	expected := make(map[string]string, len(kitFiles))
	scanner := bufio.NewScanner(io.LimitReader(manifest, maxChecksumManifestBytes+1))
	for scanner.Scan() {
		matches := checksumLine.FindStringSubmatch(scanner.Text())
		if matches == nil {
			return errors.New("checksum manifest contains an invalid line")
		}
		if _, duplicate := expected[matches[2]]; duplicate {
			return errors.New("checksum manifest contains a duplicate entry")
		}
		expected[matches[2]] = matches[1]
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(kitFiles))
	for _, name := range kitFiles {
		allowed[name] = struct{}{}
		if _, exists := expected[name]; !exists {
			return fmt.Errorf("checksum manifest is missing %s", name)
		}
	}
	for name := range expected {
		if _, exists := allowed[name]; !exists {
			return fmt.Errorf("checksum manifest contains unexpected entry %s", name)
		}
	}
	for _, name := range kitFiles {
		actual, err := hashRegularFile(filepath.Join(base, name))
		if err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
		if actual != expected[name] {
			return fmt.Errorf("checksum mismatch: %s", name)
		}
	}
	return nil
}

func hashRegularFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
