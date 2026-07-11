package agentreport

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

func TestCheckAcceptsPassingHomePair(t *testing.T) {
	directory := t.TempDir()
	listener := writeReport(t, directory, "H-A.json", passingReport("H-A", "listen", "RIGHT"))
	connector := writeReport(t, directory, "H-B.json", passingReport("H-B", "connect", "LEFT"))
	result := Check([]string{listener, connector})
	if !result.Valid || !result.PairPassed || len(result.Issues) != 0 {
		t.Fatalf("Check() = %+v", result)
	}
	if result.Files[0].File != "H-A.json" {
		t.Fatalf("file = %q", result.Files[0].File)
	}
}

func TestCheckRejectsMixedLocationAndSameEdge(t *testing.T) {
	directory := t.TempDir()
	first := writeReport(t, directory, "H-A.json", passingReport("H-A", "listen", "RIGHT"))
	second := writeReport(t, directory, "S-B.json", passingReport("S-B", "connect", "RIGHT"))
	result := Check([]string{first, second})
	if !result.Valid || result.PairPassed || !hasIssue(result.Issues, "mixed_location_pair") || !hasIssue(result.Issues, "non_opposite_edges") {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsIncompleteOrEmergencyReport(t *testing.T) {
	directory := t.TempDir()
	report := passingReport("H-A", "listen", "RIGHT")
	report.Passed = false
	report.StopReason = "emergency_release"
	path := writeReport(t, directory, "emergency.json", report)
	result := Check([]string{path})
	if result.Valid || result.PairPassed || !hasIssue(result.Files[0].Issues, "session_not_passed") {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsReportWithoutCrashGuardEvidence(t *testing.T) {
	directory := t.TempDir()
	report := passingReport("H-A", "listen", "RIGHT")
	report.CrashGuard = lab.CrashGuardEvidence{}
	path := writeReport(t, directory, "missing-guard.json", report)
	result := Check([]string{path})
	if result.Valid || !hasIssue(result.Files[0].Issues, "missing_crash_guard_evidence") {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"krasis.lab.agent-session/v4","surprise":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Check([]string{path})
	if result.Valid || !hasIssue(result.Files[0].Issues, "invalid_json") {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsInvalidOrMixedLivenessPolicy(t *testing.T) {
	directory := t.TempDir()
	invalidReport := passingReport("H-A", "listen", "RIGHT")
	invalidReport.PeerTimeoutMillis = invalidReport.HeartbeatMillis
	invalid := writeReport(t, directory, "invalid.json", invalidReport)
	result := Check([]string{invalid})
	if result.Valid || !hasIssue(result.Files[0].Issues, "invalid_liveness_policy") {
		t.Fatalf("Check(invalid policy) = %+v", result)
	}

	first := writeReport(t, directory, "H-A.json", passingReport("H-A", "listen", "RIGHT"))
	secondReport := passingReport("H-B", "connect", "LEFT")
	secondReport.HeartbeatMillis = 750
	secondReport.PeerTimeoutMillis = 4_000
	second := writeReport(t, directory, "H-B.json", secondReport)
	result = Check([]string{first, second})
	if !result.Valid || result.PairPassed || !hasIssue(result.Issues, "mixed_heartbeat_interval") || !hasIssue(result.Issues, "mixed_peer_timeout") {
		t.Fatalf("Check(mixed policy) = %+v", result)
	}
}

func passingReport(alias, role, edge string) lab.AgentSessionReport {
	return lab.AgentSessionReport{
		SchemaVersion: lab.AgentSessionSchemaVersion, DeviceAlias: alias, Role: role, Edge: edge,
		CollectedAt:   time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC),
		BuildRevision: "abc123", RequestedMillis: 120_000, HeartbeatMillis: 500, PeerTimeoutMillis: 3_000,
		Completed: true, Passed: true, StopReason: "duration_elapsed",
		CrashGuard:       lab.CrashGuardEvidence{Started: true, CleanShutdown: true, ReportFile: alias + "-guard.json"},
		Supervisor:       lab.SupervisorEvidence{FinalState: "STOPPED", ActiveCount: 1},
		DestinationInput: lab.InputEvidence{Entries: 1, PointerMoves: 1, ReturnArms: 1},
	}
}

func writeReport(t *testing.T, directory, name string, report lab.AgentSessionReport) string {
	t.Helper()
	if report.CrashGuard.ReportFile != "" {
		guard := guardreport.Report{
			SchemaVersion: guardreport.SchemaVersion, DeviceAlias: report.DeviceAlias,
			CollectedAt: report.CollectedAt, BuildRevision: report.BuildRevision,
			Reason: string(crashguard.ExitDisarm), Completed: true, Passed: true,
		}
		guardPayload, err := json.Marshal(guard)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, report.CrashGuard.ReportFile), guardPayload, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hasIssue(issues []string, want string) bool {
	for _, issue := range issues {
		if issue == want {
			return true
		}
	}
	return false
}
