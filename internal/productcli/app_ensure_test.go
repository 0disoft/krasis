package productcli

import (
	"context"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAppRecoversOnlyMissingUnpausedAndUnownedAgent(t *testing.T) {
	base, bundle := t.TempDir(), t.TempDir()
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte("MZ-test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := launchrecord.Register(base, filepath.Join(bundle, "krasis.exe"), 186, "test"); err != nil {
		t.Fatal(err)
	}
	launches := 0
	deps := dependencies{localAppData: base, openClient: func(string) (apiClient, error) { return nil, localapi.ErrUnavailable }, agentAvailable: func() (bool, error) { return true, nil }, launch: func(string) error { launches++; return nil }}
	if r, code := ensureApp(context.Background(), deps); code != ExitOK || r.Code != "launch_requested" || launches != 1 {
		t.Fatal(r, code, launches)
	}
	deps.agentAvailable = func() (bool, error) { return false, nil }
	if r, code := ensureApp(context.Background(), deps); code != ExitOK || r.Code != "agent_present" || launches != 1 {
		t.Fatal(r, code)
	}
	deps.openClient = func(string) (apiClient, error) { return &stubClient{}, nil }
	if r, code := ensureApp(context.Background(), deps); code != ExitOK || r.Code != "agent_present" || launches != 1 {
		t.Fatal(r, code)
	}
	if err := appsettings.PauseRecovery(base, true); err != nil {
		t.Fatal(err)
	}
	deps.openClient = func(string) (apiClient, error) { t.Fatal("opened while paused"); return nil, nil }
	if r, code := ensureApp(context.Background(), deps); code != ExitOK || r.Code != "recovery_disabled" || launches != 1 {
		t.Fatal(r, code)
	}
	if err := appsettings.PauseRecovery(base, false); err != nil {
		t.Fatal(err)
	}
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: false, AutoUpdate: true}); err != nil {
		t.Fatal(err)
	}
	if r, code := ensureApp(context.Background(), deps); code != ExitOK || r.Code != "recovery_disabled" || launches != 1 {
		t.Fatal(r, code)
	}
}
