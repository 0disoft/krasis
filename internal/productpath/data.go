// Package productpath resolves the per-user parent of the Krasis data directory.
package productpath

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// LocalData preserves LOCALAPPDATA on Windows and uses the XDG data directory
// on Linux. The returned directory is the parent of the existing Krasis root.
// Resolution is read-only so help, version and status never create user data.
func LocalData() (string, error) {
	if runtime.GOOS != "linux" {
		if path := os.Getenv("LOCALAPPDATA"); path != "" {
			return path, nil
		}
		return "", errors.New("LOCALAPPDATA is required")
	}
	if path := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return "", errors.New("an absolute HOME or XDG_DATA_HOME is required")
	}
	return filepath.Join(home, ".local", "share"), nil
}

// PrepareLocalData creates the XDG parent only when starting the Linux agent.
// Product security policies are applied to its Krasis child by productidentity.
func PrepareLocalData() (string, error) {
	path, err := LocalData()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "linux" {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", err
		}
	}
	return path, nil
}
