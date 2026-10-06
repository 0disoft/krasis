package productlog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestRecoveryResumesInterruptedCorruptSave(t *testing.T) {
	for _, archived := range []bool{false, true} {
		base := t.TempDir()
		failed := RecoverySummary{Time: "2026-10-07T00:00:00Z", Code: "launch_failed"}
		if err := SaveRecovery(base, failed); err != nil {
			t.Fatal(err)
		}
		dir, err := productidentity.PrepareDiagnostics(base)
		if err != nil {
			t.Fatal(err)
		}
		if err = dir.PromoteNoReplace("recovery-latest.json", recoveryPendingName); err != nil {
			t.Fatal(err)
		}
		f, err := dir.CreateExclusive("recovery-latest.json", 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte("{broken}")); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		if archived {
			if err = archiveCorruptRecovery(dir); err != nil {
				t.Fatal(err)
			}
		}
		dir.Close()
		if _, err = RecoveryReceipt(base); !errors.Is(err, errRecoverySavePending) {
			t.Fatal("unfinished save reported as final", err)
		}
		if err = SaveRecovery(base, RecoverySummary{Time: "2026-10-07T00:01:00Z", Code: "agent_present"}); err != nil {
			t.Fatal(err)
		}
		got, err := RecoveryReceipt(base)
		if err != nil || got == nil || got.LastFailure == nil || got.LastFailure.Code != "launch_failed" {
			t.Fatal(got, err)
		}
		paths, _ := productidentity.ResolveWindowsPaths(base)
		if _, err = os.Stat(filepath.Join(paths.Diagnostics, recoveryPendingName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("checkpoint not completed", err)
		}
	}
}

func TestRecoveryPreservesHistoryWarningAfterLegacyArchiveGap(t *testing.T) {
	base := t.TempDir()
	if err := SaveRecovery(base, RecoverySummary{Time: "2026-10-07T00:00:00Z", Code: "agent_present"}); err != nil {
		t.Fatal(err)
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	if err = archiveCorruptRecovery(dir); err != nil {
		t.Fatal(err)
	}
	dir.Close()
	if err = SaveRecovery(base, RecoverySummary{Time: "2026-10-07T00:01:00Z", Code: "agent_present"}); err != nil {
		t.Fatal(err)
	}
	got, err := RecoveryReceipt(base)
	if err != nil || got == nil || !got.HistoryIncomplete {
		t.Fatal("archive gap hidden", got, err)
	}
}
