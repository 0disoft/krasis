//go:build windows

package inputworker

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

func TestBootAndInstallerStartRequireTheCorrectTransactionPhase(t *testing.T) {
	previous, target := installTestImages("previous"), installTestImages("target")
	for _, phase := range []installPhase{installStage, installStop, installReplace, installStart, installCommit, installRollbackStop, installRollbackRestore, installRollbackStart, installRollbackCommit} {
		journal := &installJournal{Schema: "krasis.input.transaction/v1", Phase: phase, Previous: installSnapshot{Present: true, Running: true, Images: previous}, Target: target}
		for _, image := range []installImages{previous, target, installTestImages("unrelated")} {
			for _, transaction := range []bool{false, true} {
				args := []string{ServiceName}
				if transaction {
					args = append(args, installServiceStartArgument)
				}
				want := phase == installStart && transaction && image == target || phase == installCommit && !transaction && image == target || (phase == installRollbackStart || phase == installRollbackCommit) && !transaction && image == previous
				if serviceStartAllowed(args, image, journal) != want {
					t.Fatal("startup bypassed transaction ownership", phase, transaction, image)
				}
			}
		}
	}
	if !serviceStartAllowed([]string{ServiceName}, target, nil) || serviceStartAllowed([]string{ServiceName, installServiceStartArgument}, target, nil) {
		t.Fatal("normal reboot not separated from installer startup")
	}
	if serviceStartAllowed([]string{ServiceName}, target, &installJournal{}) || serviceStartAllowed([]string{"other"}, target, nil) {
		t.Fatal("invalid journal or service accepted")
	}
}

func TestServiceStartupRefusalPrecedesRunningStateAndInput(t *testing.T) {
	called := false
	handler := serviceHandler{check: func([]string) error { return ErrRecovery }, run: func(context.Context) error { called = true; return nil }}
	updates := make(chan svc.Status, 4)
	specific, code := handler.Execute([]string{ServiceName}, nil, updates)
	if !specific || code == 0 || called {
		t.Fatal("uncommitted service ran")
	}
	for len(updates) > 0 {
		if (<-updates).State == svc.Running {
			t.Fatal("uncommitted service advertised readiness")
		}
	}
}

func TestAutomaticSCMConfigKeepsQueryOnlyUserPermissions(t *testing.T) {
	command := `"C:\Program Files\Krasis\Input\krasis-input.exe" --service`
	config := installServiceConfig{typeID: windows.SERVICE_WIN32_OWN_PROCESS, start: windows.SERVICE_AUTO_START, errorControl: windows.SERVICE_ERROR_NORMAL, binary: command, account: "LocalSystem"}
	if !validInstallServiceConfig(config, command) {
		t.Fatal("automatic committed service rejected")
	}
	config.binary = `C:\Users\Public\input.exe`
	if validInstallServiceConfig(config, command) {
		t.Fatal("automatic service accepted an arbitrary image")
	}
}

func TestCommittedInstallationRestoresReadinessAfterReboot(t *testing.T) {
	images := installTestImages("committed")
	b := &installTestBackend{journal: &installJournal{Schema: "krasis.input.transaction/v1", Phase: installCommit, Target: images}, state: installSnapshot{Present: true, Images: images}}
	if err := recoverInputInstallation(b); err != nil || !b.state.Running || b.journal != nil {
		t.Fatal("committed reboot recovery failed", err, b.state, b.journal)
	}
	blocked := &installTestBackend{journal: &installJournal{Schema: "krasis.input.transaction/v1", Phase: installCommit, Target: images}, state: installSnapshot{Present: true, Images: installTestImages("other")}}
	if recoverInputInstallation(blocked) == nil || blocked.state.Running || blocked.journal == nil {
		t.Fatal("changed committed image started")
	}
}
