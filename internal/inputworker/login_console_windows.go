//go:build windows

package inputworker

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/loginagent"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// Launch primitive for the future restricted connection runner. It deliberately
// stays private and unwired until the child bootstrap and input retirement
// protocol are implemented. A scheduling hash alone is never launch authority.
func launchLoginConsole(ctx context.Context, installation *Installation, session uint32, profile [32]byte, prepare func(windows.Handle, uint32) error) (*ConsoleWorker, error) {
	if ctx == nil || ctx.Err() != nil || installation == nil || session == 0 || session == ^uint32(0) || profile == ([32]byte{}) || prepare == nil {
		return nil, ErrAuthority
	}
	_, digest, err := installation.digests()
	if err != nil || !fixedInstalledInputProcess(digest, false, installation) {
		return nil, ErrAuthority
	}
	managed, err := svc.IsWindowsService()
	if err != nil || !managed {
		return nil, ErrAuthority
	}
	authorized := func() bool {
		if ctx.Err() != nil || !fixedInstalledInputProcess(digest, false, installation) {
			return false
		}
		console, err := loginagent.ReadConsole()
		if err != nil || console != (loginagent.Console{Session: session, State: loginagent.BeforeLogin}) {
			return false
		}
		now := time.Now().UTC()
		grant, present, err := ReadLoginGrant(now)
		if err != nil || !present {
			return false
		}
		selected, err := grant.Selection(now)
		if err != nil || !selected.Enabled || selected.Profile != profile {
			return false
		}
		if _, err := loginagent.ReadGrantConnection(grant, now); err != nil {
			return false
		}
		// Credential reads can span a logon/session switch.
		console, err = loginagent.ReadConsole()
		return err == nil && console == (loginagent.Console{Session: session, State: loginagent.BeforeLogin})
	}
	return launchManagedConsoleMode(ctx, digest, prepare, installation, loginConsole, authorized)
}
