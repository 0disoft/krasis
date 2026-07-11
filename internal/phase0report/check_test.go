package phase0report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/guardreport"
	"github.com/0disoft/krasis/internal/lab"
)

func TestCheckAcceptsSequentialLocationEvidence(t *testing.T) {
	directory := t.TempDir()
	build := "abc123"
	duration := int64(120_000)
	homeTime := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	studyTime := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)
	home := []string{
		writeAgent(t, directory, "H-A", "listen", "RIGHT", build, duration, homeTime),
		writeAgent(t, directory, "H-B", "connect", "LEFT", build, duration, homeTime),
	}
	study := []string{
		writeAgent(t, directory, "S-A", "listen", "RIGHT", build, duration, studyTime),
		writeAgent(t, directory, "S-B", "connect", "LEFT", build, duration, studyTime),
	}
	recoveries := []string{
		writeRecovery(t, directory, "H-A", build, homeTime), writeRecovery(t, directory, "H-B", build, homeTime),
		writeRecovery(t, directory, "S-A", build, studyTime), writeRecovery(t, directory, "S-B", build, studyTime),
	}
	result := Check(home, study, recoveries)
	if !result.Passed || len(result.Issues) != 0 {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsMixedBuildAndDuplicateRecovery(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	home := []string{
		writeAgent(t, directory, "H-A", "listen", "RIGHT", "build-a", 1000, now),
		writeAgent(t, directory, "H-B", "connect", "LEFT", "build-a", 1000, now),
	}
	study := []string{
		writeAgent(t, directory, "S-A", "listen", "RIGHT", "build-b", 1000, now),
		writeAgent(t, directory, "S-B", "connect", "LEFT", "build-b", 1000, now),
	}
	recoveries := []string{
		writeRecoveryNamed(t, directory, "recovery-1.json", "H-A", "build-a", now),
		writeRecoveryNamed(t, directory, "recovery-2.json", "H-A", "build-a", now),
		writeRecoveryNamed(t, directory, "recovery-3.json", "S-A", "build-b", now),
		writeRecoveryNamed(t, directory, "recovery-4.json", "S-B", "build-b", now),
	}
	result := Check(home, study, recoveries)
	if result.Passed || !hasIssue(result.Issues, "mixed_agent_build_revision") || !hasIssue(result.Issues, "duplicate_recovery_device") || !hasIssue(result.Issues, "missing_recovery_device") {
		t.Fatalf("Check() = %+v", result)
	}
}

func writeAgent(t *testing.T, directory, alias, role, edge, build string, duration int64, collectedAt time.Time) string {
	t.Helper()
	guardName := alias + "-disarm.json"
	writeJSON(t, filepath.Join(directory, guardName), guardreport.Report{
		SchemaVersion: guardreport.SchemaVersion, DeviceAlias: alias, CollectedAt: collectedAt,
		BuildRevision: build, Reason: string(crashguard.ExitDisarm), Completed: true, Passed: true,
	})
	report := lab.AgentSessionReport{
		SchemaVersion: lab.AgentSessionSchemaVersion, DeviceAlias: alias, Role: role, Edge: edge,
		CollectedAt: collectedAt, BuildRevision: build, RequestedMillis: duration,
		Completed: true, Passed: true, StopReason: "duration_elapsed",
		CrashGuard:       lab.CrashGuardEvidence{Started: true, CleanShutdown: true, ReportFile: guardName},
		Supervisor:       lab.SupervisorEvidence{FinalState: "STOPPED", ActiveCount: 1},
		DestinationInput: lab.InputEvidence{Entries: 1, PointerMoves: 1, ReturnArms: 1},
	}
	path := filepath.Join(directory, alias+"-agent.json")
	writeJSON(t, path, report)
	return path
}

func writeRecovery(t *testing.T, directory, alias, build string, collectedAt time.Time) string {
	t.Helper()
	return writeRecoveryNamed(t, directory, alias+"-recovery.json", alias, build, collectedAt)
}

func writeRecoveryNamed(t *testing.T, directory, name, alias, build string, collectedAt time.Time) string {
	t.Helper()
	path := filepath.Join(directory, name)
	writeJSON(t, path, guardreport.Report{
		SchemaVersion: guardreport.SchemaVersion, DeviceAlias: alias, CollectedAt: collectedAt,
		BuildRevision: build, Reason: string(crashguard.ExitParentEOF),
		Attempted: 1, Released: 1, Completed: true, Passed: true,
	})
	return path
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasIssue(issues []string, want string) bool {
	for _, issue := range issues {
		if issue == want {
			return true
		}
	}
	return false
}
