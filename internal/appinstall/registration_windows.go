//go:build windows

package appinstall

import (
	"errors"

	"github.com/0disoft/krasis/internal/launchrecord"
)

// Migrate the owned task while the previous launch record still proves its
// origin. Only then publish another generation, which replaces that history.
// A failed post-publication registration is repairable by the same preflight on
// the next attempt. Task ownership checks remain in appsettings.
func registerInstalledLaunch(base, ui string, generation int, revision string, enabled bool, recovery, startup func(string, bool) error) error {
	if _, err := launchrecord.Load(base); err == nil {
		if err := recovery(base, enabled); err != nil {
			return err
		}
	} else if !errors.Is(err, launchrecord.ErrMissing) {
		return err
	}
	if err := launchrecord.Register(base, ui, generation, revision); err != nil {
		return err
	}
	if err := recovery(base, enabled); err != nil {
		return err
	}
	return startup(base, enabled)
}
