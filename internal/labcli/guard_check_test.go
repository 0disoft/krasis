package labcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/guardreport"
)

func TestRunCrashRecoveryCheckJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	payload, err := json.Marshal(guardreport.Report{
		SchemaVersion: guardreport.SchemaVersion, DeviceAlias: "H-A",
		CollectedAt: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), BuildRevision: "abc123",
		Reason: string(crashguard.ExitParentEOF), Attempted: 1, Released: 1, Completed: true, Passed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(t.Context(), []string{"crash-recovery-check", "--json", path}, &stdout, &stderr, nil)
	if code != exitSuccess || !strings.Contains(stdout.String(), `"valid":true`) || stderr.Len() != 0 {
		t.Fatalf("Run() = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestRunPhase0CheckRequiresAllReportGroups(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run(t.Context(), []string{"phase0-check", "--home", "H-A.json"}, &stdout, &stderr, nil)
	if code != exitUsage || !strings.Contains(stderr.String(), "usage: krasis-lab phase0-check") {
		t.Fatalf("Run() = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
}
