package guardreport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/crashguard"
)

func TestCheckAcceptsParentEOFRecovery(t *testing.T) {
	path := writeGuardReport(t, passingGuardReport(crashguard.ExitParentEOF, 2))
	result := Check(path, Expectation{Reason: crashguard.ExitParentEOF, RequireCleanBuild: true, RequireRecovery: true})
	if !result.Valid || len(result.Issues) != 0 {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsEmptyRecoveryAndDirtyBuild(t *testing.T) {
	report := passingGuardReport(crashguard.ExitParentEOF, 0)
	report.BuildModified = true
	result := Check(writeGuardReport(t, report), Expectation{Reason: crashguard.ExitParentEOF, RequireCleanBuild: true, RequireRecovery: true})
	if result.Valid || !contains(result.Issues, "no_recovery_attempt") || !contains(result.Issues, "modified_build") {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsDisarmThatHadToRecoverInput(t *testing.T) {
	result := Check(writeGuardReport(t, passingGuardReport(crashguard.ExitDisarm, 1)), Expectation{Reason: crashguard.ExitDisarm})
	if result.Valid || !contains(result.Issues, "unexpected_disarm_recovery") {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"krasis.lab.crash-guard/v1","surprise":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Check(path, Expectation{})
	if result.Valid || !contains(result.Issues, "invalid_json") {
		t.Fatalf("Check() = %+v", result)
	}
}

func passingGuardReport(reason crashguard.ExitReason, attempted int) Report {
	return Report{
		SchemaVersion: SchemaVersion, DeviceAlias: "H-A", CollectedAt: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC),
		BuildRevision: "abc123", Reason: string(reason), Attempted: attempted, Released: attempted, Completed: true, Passed: true,
	}
}

func writeGuardReport(t *testing.T, report Report) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "guard.json")
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
