package guardreport

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/lab"
)

const maxReportSize = 1 << 20

type Expectation struct {
	Reason            crashguard.ExitReason
	DeviceAlias       string
	BuildRevision     string
	RequireCleanBuild bool
	RequireRecovery   bool
}

type CheckResult struct {
	SchemaVersion string   `json:"schemaVersion"`
	File          string   `json:"file"`
	Valid         bool     `json:"valid"`
	Issues        []string `json:"issues"`
	DeviceAlias   string   `json:"deviceAlias,omitempty"`
	BuildRevision string   `json:"buildRevision,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	Attempted     int      `json:"attempted"`
	Released      int      `json:"released"`
	Remaining     int      `json:"remaining"`
}

func Check(path string, expectation Expectation) CheckResult {
	result := CheckResult{SchemaVersion: "krasis.lab.crash-guard-check/v1", File: filepath.Base(path), Issues: []string{}}
	report, issues := read(path)
	result.Issues = append(result.Issues, issues...)
	result.DeviceAlias = report.DeviceAlias
	result.BuildRevision = report.BuildRevision
	result.Reason = report.Reason
	result.Attempted = report.Attempted
	result.Released = report.Released
	result.Remaining = report.Remaining
	if len(issues) == 0 {
		result.Issues = append(result.Issues, validate(report, expectation)...)
	}
	result.Valid = len(result.Issues) == 0
	return result
}

func read(path string) (Report, []string) {
	file, err := os.Open(path)
	if err != nil {
		return Report{}, []string{"open_failed"}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Report{}, []string{"stat_failed"}
	}
	if !info.Mode().IsRegular() {
		return Report{}, []string{"not_regular_file"}
	}
	if info.Size() > maxReportSize {
		return Report{}, []string{"report_too_large"}
	}
	var report Report
	decoder := json.NewDecoder(io.LimitReader(file, maxReportSize+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return Report{}, []string{"invalid_json"}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Report{}, []string{"trailing_json"}
		}
		return Report{}, []string{"invalid_json"}
	}
	return report, nil
}

func validate(report Report, expectation Expectation) []string {
	issues := make([]string, 0)
	if report.SchemaVersion != SchemaVersion {
		issues = append(issues, "unsupported_schema")
	}
	if !lab.ValidProbeDeviceAlias(report.DeviceAlias) {
		issues = append(issues, "invalid_device_alias")
	}
	if report.CollectedAt.IsZero() {
		issues = append(issues, "missing_collected_at")
	}
	if report.BuildRevision == "" || report.BuildRevision == "unknown" {
		issues = append(issues, "unknown_build_revision")
	}
	if report.Reason != string(crashguard.ExitParentEOF) && report.Reason != string(crashguard.ExitDisarm) && report.Reason != string(crashguard.ExitProtocolError) {
		issues = append(issues, "invalid_reason")
	}
	if report.Attempted < 0 || report.Released < 0 || report.Remaining < 0 || report.Released > report.Attempted || report.Remaining != report.Attempted-report.Released {
		issues = append(issues, "invalid_release_counts")
	}
	if !report.Completed || !report.Passed || report.FailureCode != "" || report.Remaining != 0 {
		issues = append(issues, "guard_not_passed")
	}
	if expectation.Reason != "" && report.Reason != string(expectation.Reason) {
		issues = append(issues, "unexpected_reason")
	}
	if expectation.DeviceAlias != "" && report.DeviceAlias != expectation.DeviceAlias {
		issues = append(issues, "device_mismatch")
	}
	if expectation.BuildRevision != "" && report.BuildRevision != expectation.BuildRevision {
		issues = append(issues, "build_mismatch")
	}
	if expectation.RequireCleanBuild && report.BuildModified {
		issues = append(issues, "modified_build")
	}
	if expectation.RequireRecovery && report.Attempted == 0 {
		issues = append(issues, "no_recovery_attempt")
	}
	if expectation.Reason == crashguard.ExitDisarm && report.Attempted != 0 {
		issues = append(issues, "unexpected_disarm_recovery")
	}
	return issues
}
