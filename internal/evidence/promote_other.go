//go:build !windows

package evidence

import (
	"errors"
	"os"
)

func promoteNoReplace(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrOutputExists
		}
		return err
	}
	return os.Remove(source)
}
