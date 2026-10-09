package productpanel

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestAutomaticStartupRechecksStopAfterStartupLockAndPreparation(t *testing.T) {
	for _, stopAt := range []string{"lock", "prepare"} {
		base := t.TempDir()
		token, err := appsettings.RecoveryToken(base)
		if err != nil {
			t.Fatal(err)
		}
		stop := func() {
			if err := appsettings.PauseRecovery(base, true); err != nil {
				t.Fatal(err)
			}
		}
		s := Startup{
			Lock: func() (io.Closer, error) {
				if stopAt == "lock" {
					stop()
				}
				return closerStub{}, nil
			},
			CheckStart: func() error {
				if !appsettings.RecoveryCurrent(base, token) {
					return errors.New("superseded")
				}
				return nil
			},
			Open:      func() (RuntimeClient, error) { return nil, localapi.ErrUnavailable },
			Available: func() (bool, error) { return true, nil }, Verify: func() error { return nil },
			BeforeStart: func(context.Context) error { stop(); return nil },
			Start:       func() error { t.Fatal("started after stop"); return nil },
		}
		if err := s.Ensure(context.Background()); err == nil {
			t.Fatal("stale automatic start accepted")
		}
		paused, err := appsettings.RecoveryPaused(base)
		if err != nil || !paused {
			t.Fatal(paused, err)
		}
	}
}
