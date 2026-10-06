//go:build !windows

package launchrecord

import (
	"os"
	"path/filepath"
)

func LiveDirectories() ([]string, error) {
	self, err := os.Executable()
	return []string{filepath.Dir(self)}, err
}
