//go:build !windows

package inputworker

import (
	"github.com/0disoft/krasis/internal/application"
	"io"
)

func adoptPortableFixtureOwnership(args []string) (io.Closer, error) {
	if len(args) != 0 {
		return nil, ErrAuthority
	}
	return nil, nil
}

func servePortableFixtureGuardian([]string, application.ReleasePort) error { return ErrAuthority }
