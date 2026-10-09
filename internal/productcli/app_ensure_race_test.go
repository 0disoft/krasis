package productcli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestEnsureAppDiscardsRequestAfterSafeStopOrLaterRestart(t *testing.T) {
	for _, restart := range []bool{false, true} {
		base, bundle := t.TempDir(), t.TempDir()
		for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
			if err := os.WriteFile(filepath.Join(bundle, name), []byte("MZ-test"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := launchrecord.Register(base, filepath.Join(bundle, "krasis.exe"), 257, "test"); err != nil {
			t.Fatal(err)
		}
		deps := dependencies{localAppData: base, openClient: func(string) (apiClient, error) { return nil, localapi.ErrUnavailable },
			agentAvailable: func() (bool, error) {
				if err := appsettings.PauseRecovery(base, true); err != nil {
					t.Fatal(err)
				}
				if restart {
					if err := appsettings.PauseRecovery(base, false); err != nil {
						t.Fatal(err)
					}
				}
				return true, nil
			}, launch: func(string) error { t.Fatal("stale recovery launched"); return nil },
		}
		if result, code := ensureApp(context.Background(), deps); code != ExitOK || result.Code != "recovery_disabled" {
			t.Fatal(result, code)
		}
	}
}
