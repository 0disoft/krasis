//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/loginagent"
	"github.com/0disoft/krasis/internal/wininput"
)

// RunInstalledLoginConnection has no caller-selected target or runtime callback.
// The protected service pipe supplies the immutable session and grant binding.
// Before login it exposes only the pinned peer's ordinary input protocol.
func RunInstalledLoginConnection(ctx context.Context, installation *Installation) error {
	return runInstalledLoginWorker(ctx, installation, func(lifetime context.Context, session uint32, profile [32]byte, authenticate func() error) error {
		authorize := func() error {
			if err := authenticate(); err != nil {
				return err
			}
			return authorizeLoginProfile(session, profile)
		}
		if err := authorize(); err != nil {
			return err
		}
		grant, present, err := ReadLoginGrant(time.Now().UTC())
		if err != nil || !present {
			return ErrAuthority
		}
		live, err := loginagent.ReadGrantConnection(grant, time.Now().UTC())
		if err != nil {
			return err
		}
		geometry := wininput.CurrentDisplayGeometry()
		if !geometry.Valid() || live.Profile.Rearm() >= geometry.Width {
			return wininput.ErrDisplayGeometryChanged
		}
		hook, err := wininput.NewLoginSourceHook(authorize)
		if err != nil {
			return err
		}
		hook.SetBoundaryGuard(func() error {
			if wininput.CurrentDisplayGeometry() != geometry {
				return wininput.ErrDisplayGeometryChanged
			}
			return authorize()
		})
		report, err := RunLoginInput(lifetime, authorize, func(owned context.Context, port application.InputPort) error {
			input, err := wininput.NewInputAdapterWithInjection(live.Profile.Rearm(), geometry.Width, port)
			if err != nil {
				return err
			}
			return loginagent.RunConnection(owned, session, profile, loginagent.ConnectionDependencies{
				ReadGrant: ReadLoginGrant, Hook: hook, Input: input, Geometry: geometry,
			})
		})
		if report.Recovery.Remaining != 0 {
			return errors.Join(err, ErrRecovery)
		}
		return err
	})
}
