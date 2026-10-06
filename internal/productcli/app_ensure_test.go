package productcli

import (
	"context"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
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
	deps.openClient = func(string) (apiClient, error) { return &stubClient{status: localapi.Status{State: "LOCAL"}}, nil }
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

func TestEnsureAppRecordsObservedStateWithoutRestartingAnOwnedAgent(t *testing.T) {
	for _, state := range []string{"LOCAL", "CONNECTED", "CONNECTING", "RECONNECTING", "DISCONNECTING", "FAULTED", "private detail"} {
		t.Run(state, func(t *testing.T) {
			base := t.TempDir()
			deps := dependencies{localAppData: base,
				openClient:     func(string) (apiClient, error) { return &stubClient{status: localapi.Status{State: state}}, nil },
				agentAvailable: func() (bool, error) { t.Fatal("queried lease after status response"); return false, nil },
				launch:         func(string) error { t.Fatal("relaunched a responding agent"); return nil },
			}
			result, code := ensureApp(context.Background(), deps)
			wantCode, wantState, exit := "agent_present", state, ExitOK
			if state == "FAULTED" {
				wantCode, exit = "agent_faulted", ExitUnsafeState
			}
			if state == "private detail" {
				wantCode, wantState, exit = "agent_unverified", "UNKNOWN", ExitUnsafeState
			}
			if result.Code != wantCode || result.AgentState != wantState || code != exit {
				t.Fatal(result, code)
			}
			receipt, err := productlog.RecoveryReceipt(base)
			if err != nil || receipt == nil || receipt.Code != wantCode || receipt.AgentState != wantState {
				t.Fatal(receipt, err)
			}
		})
	}
}
