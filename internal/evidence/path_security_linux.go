//go:build linux

package evidence

import (
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/securefs"
)

func validateOutputDirectory(path string) error {
	directory, err := securefs.OpenReadDirectory(path)
	if err != nil {
		if errors.Is(err, securefs.ErrUnsafePath) {
			return fmt.Errorf("%w: %w", ErrUnsafeOutputPath, err)
		}
		return fmt.Errorf("inspect evidence output directory: %w", err)
	}
	return directory.Close()
}
