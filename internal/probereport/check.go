package probereport

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/lab"
)

const (
	SchemaVersion = "krasis.lab.probe-check/v1"
	maxReportSize = 1 << 20
)

type Report struct {
	SchemaVersion string       `json:"schemaVersion"`
	InputCount    int          `json:"inputCount"`
	Valid         bool         `json:"valid"`
	Comparable    bool         `json:"comparable"`
	Issues        []string     `json:"issues"`
	Files         []FileResult `json:"files"`
}

type FileResult struct {
	File                    string   `json:"file"`
	Valid                   bool     `json:"valid"`
	Issues                  []string `json:"issues"`
	DeviceAlias             string   `json:"deviceAlias,omitempty"`
	BuildRevision           string   `json:"buildRevision,omitempty"`
	BuildModified           bool     `json:"buildModified,omitempty"`
	RequestedDurationMillis int64    `json:"requestedDurationMillis,omitempty"`
	Completed               bool     `json:"completed,omitempty"`
}

func Check(paths []string) Report {
	report := Report{
		SchemaVersion: SchemaVersion,
		InputCount:    len(paths),
		Issues:        []string{},
		Files:         make([]FileResult, 0, len(paths)),
	}
	probes := make([]lab.ProbeReport, 0, len(paths))
	for _, path := range paths {
		fileResult, probe := checkFile(path)
		report.Files = append(report.Files, fileResult)
		if fileResult.Valid {
			probes = append(probes, probe)
		}
	}

	report.Valid = len(paths) > 0 && len(probes) == len(paths)
	if !report.Valid {
		report.Issues = append(report.Issues, "invalid_report")
	}
	if len(paths) < 2 {
		return report
	}

	comparisonIssues := compare(probes, len(paths))
	report.Issues = append(report.Issues, comparisonIssues...)
	report.Comparable = report.Valid && len(comparisonIssues) == 0
	return report
}

func checkFile(path string) (FileResult, lab.ProbeReport) {
	result := FileResult{
		File:   filepath.Base(path),
		Issues: []string{},
	}
	file, err := os.Open(path)
	if err != nil {
		result.Issues = append(result.Issues, "open_failed")
		return result, lab.ProbeReport{}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		result.Issues = append(result.Issues, "stat_failed")
		return result, lab.ProbeReport{}
	}
	if !info.Mode().IsRegular() {
		result.Issues = append(result.Issues, "not_regular_file")
		return result, lab.ProbeReport{}
	}
	if info.Size() > maxReportSize {
		result.Issues = append(result.Issues, "report_too_large")
		return result, lab.ProbeReport{}
	}

	var probe lab.ProbeReport
	decoder := json.NewDecoder(io.LimitReader(file, maxReportSize+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&probe); err != nil {
		result.Issues = append(result.Issues, "invalid_json")
		return result, lab.ProbeReport{}
	}
	if err := ensureJSONEOF(decoder); err != nil {
		result.Issues = append(result.Issues, "trailing_json")
		return result, lab.ProbeReport{}
	}

	result.DeviceAlias = probe.DeviceAlias
	result.BuildRevision = probe.BuildRevision
	result.BuildModified = probe.BuildModified
	result.RequestedDurationMillis = probe.RequestedDurationMs
	result.Completed = probe.Completed
	result.Issues = validate(probe)
	result.Valid = len(result.Issues) == 0
	return result, probe
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("multiple JSON values are not allowed")
}

func validate(probe lab.ProbeReport) []string {
	issues := make([]string, 0)
	if probe.SchemaVersion != lab.ProbeSchemaVersion {
		issues = append(issues, "unsupported_probe_schema")
	}
	if !lab.ValidProbeDeviceAlias(probe.DeviceAlias) {
		issues = append(issues, "invalid_device_alias")
	}
	if probe.CollectedAt.IsZero() {
		issues = append(issues, "missing_collected_at")
	}
	if probe.BuildRevision == "" || probe.BuildRevision == "unknown" {
		issues = append(issues, "unknown_build_revision")
	}
	if probe.RequestedDurationMs <= 0 {
		issues = append(issues, "invalid_duration")
	}
	if probe.Diagnostics != nil && probe.Diagnostics.SchemaVersion != lab.DiagnoseSchemaVersion {
		issues = append(issues, "unsupported_diagnose_schema")
	}
	if probe.Capture != nil && probe.Capture.SchemaVersion != lab.CaptureSchemaVersion {
		issues = append(issues, "unsupported_capture_schema")
	}
	if probe.SelfTest != nil && probe.SelfTest.SchemaVersion != lab.SelfTestSchemaVersion {
		issues = append(issues, "unsupported_selftest_schema")
	}
	if probe.Completed {
		if probe.FailureCode != "" || probe.Diagnostics == nil || probe.Capture == nil || probe.SelfTest == nil || !probe.SelfTest.Passed {
			issues = append(issues, "inconsistent_completed_report")
		}
	} else if probe.FailureCode == "" {
		issues = append(issues, "missing_failure_code")
	}
	return issues
}

func compare(probes []lab.ProbeReport, inputCount int) []string {
	if len(probes) != inputCount {
		return nil
	}
	issues := make([]string, 0)
	aliases := make(map[string]struct{}, len(probes))
	buildRevision := probes[0].BuildRevision
	duration := probes[0].RequestedDurationMs
	for _, probe := range probes {
		if _, exists := aliases[probe.DeviceAlias]; exists {
			issues = appendUnique(issues, "duplicate_device_alias")
		}
		aliases[probe.DeviceAlias] = struct{}{}
		if probe.BuildRevision != buildRevision {
			issues = appendUnique(issues, "mixed_build_revision")
		}
		if probe.BuildModified {
			issues = appendUnique(issues, "modified_build")
		}
		if probe.RequestedDurationMs != duration {
			issues = appendUnique(issues, "mixed_duration")
		}
		if !probe.Completed {
			issues = appendUnique(issues, "incomplete_report")
		}
	}
	return issues
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
