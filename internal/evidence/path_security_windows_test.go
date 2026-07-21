//go:build windows

package evidence

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestValidateJSONOutputPathRejectsReparseDirectoryComponent(t *testing.T) {
	directory := t.TempDir()
	reparseComponent := filepath.Clean(directory)
	original := getOutputPathAttributes
	getOutputPathAttributes = func(path string) (uint32, error) {
		if filepath.Clean(path) == reparseComponent {
			return outputFileAttributeReparsePoint, nil
		}
		return 0, nil
	}
	t.Cleanup(func() { getOutputPathAttributes = original })

	err := ValidateJSONOutputPath(filepath.Join(directory, "probe.json"))
	if !errors.Is(err, ErrUnsafeOutputPath) {
		t.Fatalf("ValidateJSONOutputPath() error = %v, want ErrUnsafeOutputPath", err)
	}
}
