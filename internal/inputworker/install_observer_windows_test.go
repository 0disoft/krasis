//go:build windows

package inputworker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/installationdiag"
)

type receiptInstallBackend struct {
	installTestBackend
	recoveryFailure bool
}

func (b *receiptInstallBackend) Replace(context.Context, installImages) error {
	return installNativeError("file_move", ErrAuthority, &os.PathError{Op: "rename", Path: "PRIVATE-PATH", Err: syscall.Errno(32)})
}
func (b *receiptInstallBackend) Restore(ctx context.Context, value installSnapshot) error {
	if b.recoveryFailure {
		return installNativeError("file_delete", ErrAuthority, syscall.Errno(5))
	}
	return b.installTestBackend.Restore(ctx, value)
}

func TestInstallerKeepsOriginalAndRollbackErrorsSeparate(t *testing.T) {
	for _, recoveryFailure := range []bool{false, true} {
		backend := &receiptInstallBackend{recoveryFailure: recoveryFailure}
		s := installationdiag.Summary{Time: "2026-10-02T00:00:00Z", Action: "install", Stage: "validation", Recovery: "not_attempted"}
		report := runObservedInstallation(context.Background(), backend, installTestImages("new"), s, func() error { return nil })
		got := report.Diagnostic
		if !got.Valid() || got.Stage != "replace" || got.Phase != "replace" || got.Operation != "file_move" || !got.Win32Known || got.Win32Error != 32 {
			t.Fatalf("original error lost: %+v", got)
		}
		if recoveryFailure {
			if report.Result != InstallationRecoveryRequired || got.Recovery != "required" || got.RecoveryStage != "restore" || got.RecoveryPhase != "rollback_restore" || got.RecoveryWin32Error != 5 || !got.RecoveryWin32Known {
				t.Fatalf("rollback error lost: %+v", got)
			}
		} else if report.Result != InstallationRestored || got.Recovery != "restored" || got.RecoveryWin32Known {
			t.Fatalf("restored outcome lost: %+v", got)
		}
		data, _ := json.Marshal(got)
		if strings.Contains(string(data), "PRIVATE") {
			t.Fatal("native path leaked")
		}
	}
}

func TestInstallerRetainsFailureWhenBackendCloseAlsoFails(t *testing.T) {
	backend := &receiptInstallBackend{}
	s := installationdiag.Summary{Time: "2026-10-02T00:00:00Z", Action: "install", Stage: "validation", Recovery: "not_attempted"}
	report := runObservedInstallation(context.Background(), backend, installTestImages("new"), s, func() error { return installNativeError("file_close", ErrAuthority, syscall.Errno(6)) })
	if report.Result != InstallationRecoveryRequired || report.Diagnostic.Win32Error != 32 || report.Diagnostic.CloseWin32Error != 6 || report.Diagnostic.Recovery != "restored" || !report.Diagnostic.Valid() {
		t.Fatal("close replaced original failure")
	}
	err := installNativeError("file_open", ErrAuthority, syscall.Errno(5))
	if !errors.Is(err, ErrAuthority) || err.Error() != ErrAuthority.Error() {
		t.Fatal("native error changed public authority contract")
	}
}
