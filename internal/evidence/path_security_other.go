//go:build !windows

package evidence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateOutputDirectory(path string) error {
	volume := filepath.VolumeName(path)
	current := volume + string(os.PathSeparator)
	remainder := strings.TrimPrefix(path, current)
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect evidence output path component: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symbolic link at %s", ErrUnsafeOutputPath, current)
		}
	}
	return nil
}
