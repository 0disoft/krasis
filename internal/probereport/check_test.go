package probereport

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/lab"
)

func TestCheckAcceptsComparableReports(t *testing.T) {
	directory := t.TempDir()
	first := writeProbe(t, directory, "home-a.json", validProbe("H-A", "abc123"))
	second := writeProbe(t, directory, "home-b.json", validProbe("H-B", "abc123"))

	report := Check([]string{first, second})

	if !report.Valid || !report.Comparable || len(report.Issues) != 0 {
		t.Fatalf("unexpected check report: %+v", report)
	}
	if report.Files[0].File != "home-a.json" {
		t.Fatalf("file display = %q, want basename", report.Files[0].File)
	}
}

func TestCheckRejectsMixedBuildRevisions(t *testing.T) {
	directory := t.TempDir()
	first := writeProbe(t, directory, "home-a.json", validProbe("H-A", "abc123"))
	second := writeProbe(t, directory, "home-b.json", validProbe("H-B", "def456"))

	report := Check([]string{first, second})

	if !report.Valid || report.Comparable || !contains(report.Issues, "mixed_build_revision") {
		t.Fatalf("unexpected check report: %+v", report)
	}
}

func TestCheckRejectsUnknownJSONFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"krasis.lab.probe/v1","surprise":true}`), 0o600); err != nil {
		t.Fatalf("write report: %v", err)
	}

	report := Check([]string{path})

	if report.Valid || !contains(report.Files[0].Issues, "invalid_json") {
		t.Fatalf("unexpected check report: %+v", report)
	}
}

func TestCheckRejectsTrailingJSONAndOversizedFiles(t *testing.T) {
	directory := t.TempDir()
	trailing := filepath.Join(directory, "trailing.json")
	if err := os.WriteFile(trailing, []byte(`{} {}`), 0o600); err != nil {
		t.Fatalf("write trailing report: %v", err)
	}
	oversized := filepath.Join(directory, "oversized.json")
	if err := os.WriteFile(oversized, bytes.Repeat([]byte(" "), maxReportSize+1), 0o600); err != nil {
		t.Fatalf("write oversized report: %v", err)
	}

	report := Check([]string{trailing, oversized})

	if report.Valid || !contains(report.Files[0].Issues, "trailing_json") || !contains(report.Files[1].Issues, "report_too_large") {
		t.Fatalf("unexpected check report: %+v", report)
	}
}

func TestCheckReportsEveryComparisonMismatchOnce(t *testing.T) {
	directory := t.TempDir()
	firstProbe := validProbe("H-A", "abc123")
	firstProbe.BuildModified = true
	secondProbe := validProbe("H-A", "def456")
	secondProbe.RequestedDurationMs = 10_000
	secondProbe.Completed = false
	secondProbe.FailureCode = "selftest_failed"
	secondProbe.SelfTest.Passed = false
	first := writeProbe(t, directory, "first.json", firstProbe)
	second := writeProbe(t, directory, "second.json", secondProbe)

	report := Check([]string{first, second})

	if !report.Valid || report.Comparable {
		t.Fatalf("unexpected validity: %+v", report)
	}
	for _, issue := range []string{"duplicate_device_alias", "mixed_build_revision", "modified_build", "mixed_duration", "incomplete_report"} {
		if !contains(report.Issues, issue) {
			t.Fatalf("missing issue %q in %+v", issue, report)
		}
	}
}

func TestCheckRejectsInconsistentCompletedReport(t *testing.T) {
	directory := t.TempDir()
	probe := validProbe("S-A", "abc123")
	probe.FailureCode = "selftest_failed"
	path := writeProbe(t, directory, "inconsistent.json", probe)

	report := Check([]string{path})

	if report.Valid || !contains(report.Files[0].Issues, "inconsistent_completed_report") {
		t.Fatalf("unexpected check report: %+v", report)
	}
}

func validProbe(alias, revision string) lab.ProbeReport {
	return lab.ProbeReport{
		SchemaVersion:       lab.ProbeSchemaVersion,
		DeviceAlias:         alias,
		CollectedAt:         time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC),
		BuildRevision:       revision,
		RequestedDurationMs: 30_000,
		Completed:           true,
		Diagnostics:         &lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion},
		Capture:             &lab.CaptureSnapshot{SchemaVersion: lab.CaptureSchemaVersion},
		SelfTest:            &lab.SelfTestResult{SchemaVersion: lab.SelfTestSchemaVersion, Passed: true},
	}
}

func writeProbe(t *testing.T, directory, name string, probe lab.ProbeReport) string {
	t.Helper()
	data, err := json.Marshal(probe)
	if err != nil {
		t.Fatalf("marshal probe: %v", err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	return path
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
