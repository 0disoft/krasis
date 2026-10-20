//go:build !windows

package inputworker

import "github.com/0disoft/krasis/internal/application"

func servePortableFixtureGuardian([]string, application.ReleasePort) error { return ErrAuthority }
