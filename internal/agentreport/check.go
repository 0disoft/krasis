package agentreport

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/guardreport"
	"github.com/0disoft/krasis/internal/lab"
)

const (
	SchemaVersion = "krasis.lab.agent-session-check/v3"
	maxReportSize = 1 << 20
)

type Report struct {
	SchemaVersion string       `json:"schemaVersion"`
	InputCount    int          `json:"inputCount"`
	Valid         bool         `json:"valid"`
	PairPassed    bool         `json:"pairPassed"`
	Issues        []string     `json:"issues"`
	Files         []FileResult `json:"files"`
}

type FileResult struct {
	File              string               `json:"file"`
	Valid             bool                 `json:"valid"`
	Issues            []string             `json:"issues"`
	DeviceAlias       string               `json:"deviceAlias,omitempty"`
	Role              string               `json:"role,omitempty"`
	Edge              string               `json:"edge,omitempty"`
	BuildRevision     string               `json:"buildRevision,omitempty"`
	BuildModified     bool                 `json:"buildModified"`
	RequestedMillis   int64                `json:"requestedDurationMillis"`
	HeartbeatMillis   int64                `json:"heartbeatIntervalMillis"`
	PeerTimeoutMillis int64                `json:"peerTimeoutMillis"`
	Completed         bool                 `json:"completed"`
	Passed            bool                 `json:"passed"`
	Liveness          lab.LivenessEvidence `json:"liveness"`
	GuardReport       string               `json:"guardReport,omitempty"`
	GuardValid        bool                 `json:"guardValid"`
	GuardIssues       []string             `json:"guardIssues"`
}

func Check(paths []string) Report {
	result := Report{
		SchemaVersion: SchemaVersion, InputCount: len(paths),
		Issues: []string{}, Files: make([]FileResult, 0, len(paths)),
	}
	reports := make([]lab.AgentSessionReport, 0, len(paths))
	for _, path := range paths {
		fileResult, report := checkFile(path)
		result.Files = append(result.Files, fileResult)
		if fileResult.Valid {
			reports = append(reports, report)
		}
	}
	result.Valid = len(paths) == 2 && len(reports) == 2
	if len(paths) != 2 {
		result.Issues = append(result.Issues, "pair_requires_two_reports")
	}
	if len(reports) != len(paths) {
		result.Issues = append(result.Issues, "invalid_report")
	}
	if result.Valid {
		result.Issues = append(result.Issues, comparePair(reports[0], reports[1])...)
	}
	result.PairPassed = result.Valid && len(result.Issues) == 0
	return result
}

func checkFile(path string) (FileResult, lab.AgentSessionReport) {
	result := FileResult{File: filepath.Base(path), Issues: []string{}, GuardIssues: []string{}}
	file, err := os.Open(path)
	if err != nil {
		result.Issues = append(result.Issues, "open_failed")
		return result, lab.AgentSessionReport{}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		result.Issues = append(result.Issues, "stat_failed")
		return result, lab.AgentSessionReport{}
	}
	if !info.Mode().IsRegular() {
		result.Issues = append(result.Issues, "not_regular_file")
		return result, lab.AgentSessionReport{}
	}
	if info.Size() > maxReportSize {
		result.Issues = append(result.Issues, "report_too_large")
		return result, lab.AgentSessionReport{}
	}
	var report lab.AgentSessionReport
	decoder := json.NewDecoder(io.LimitReader(file, maxReportSize+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		result.Issues = append(result.Issues, "invalid_json")
		return result, lab.AgentSessionReport{}
	}
	if err := ensureEOF(decoder); err != nil {
		result.Issues = append(result.Issues, "trailing_json")
		return result, lab.AgentSessionReport{}
	}
	result.DeviceAlias = report.DeviceAlias
	result.Role = report.Role
	result.Edge = report.Edge
	result.BuildRevision = report.BuildRevision
	result.BuildModified = report.BuildModified
	result.RequestedMillis = report.RequestedMillis
	result.HeartbeatMillis = report.HeartbeatMillis
	result.PeerTimeoutMillis = report.PeerTimeoutMillis
	result.Completed = report.Completed
	result.Passed = report.Passed
	result.Liveness = report.Liveness
	result.Issues = validate(report)
	if report.CrashGuard.ReportFile == "" || filepath.Base(report.CrashGuard.ReportFile) != report.CrashGuard.ReportFile {
		result.Issues = append(result.Issues, "invalid_guard_report_reference")
	} else {
		guardPath := filepath.Join(filepath.Dir(path), report.CrashGuard.ReportFile)
		guardResult := guardreport.Check(guardPath, guardreport.Expectation{
			Reason: crashguard.ExitDisarm, DeviceAlias: report.DeviceAlias,
			BuildRevision: report.BuildRevision, RequireCleanBuild: true,
		})
		result.GuardReport = guardResult.File
		result.GuardValid = guardResult.Valid
		result.GuardIssues = guardResult.Issues
		if !guardResult.Valid {
			result.Issues = append(result.Issues, "invalid_guard_report")
		}
	}
	result.Valid = len(result.Issues) == 0
	return result, report
}

func ensureEOF(decoder *json.Decoder) error {
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

func validate(report lab.AgentSessionReport) []string {
	issues := make([]string, 0)
	if report.SchemaVersion != lab.AgentSessionSchemaVersion {
		issues = append(issues, "unsupported_agent_session_schema")
	}
	if !lab.ValidProbeDeviceAlias(report.DeviceAlias) {
		issues = append(issues, "invalid_device_alias")
	}
	if report.Role != "listen" && report.Role != "connect" {
		issues = append(issues, "invalid_role")
	}
	if report.Edge != "LEFT" && report.Edge != "RIGHT" {
		issues = append(issues, "invalid_edge")
	}
	if report.CollectedAt.IsZero() {
		issues = append(issues, "missing_collected_at")
	}
	if report.BuildRevision == "" || report.BuildRevision == "unknown" {
		issues = append(issues, "unknown_build_revision")
	}
	if report.RequestedMillis <= 0 {
		issues = append(issues, "invalid_duration")
	}
	if report.HeartbeatMillis <= 0 || report.PeerTimeoutMillis <= report.HeartbeatMillis {
		issues = append(issues, "invalid_liveness_policy")
	}
	liveness := report.Liveness
	if liveness.ReceiveGapSamples >= liveness.HeartbeatsReceived || (liveness.ReceiveGapSamples > 0 && liveness.MaxReceiveGapMillis <= 0) {
		issues = append(issues, "invalid_liveness_evidence")
	}
	if liveness.HeartbeatsSent < 2 || liveness.HeartbeatsReceived < 2 || liveness.ReceiveGapSamples == 0 {
		issues = append(issues, "insufficient_liveness_evidence")
	}
	if liveness.MaxReceiveGapMillis >= report.PeerTimeoutMillis {
		issues = append(issues, "liveness_gap_reached_timeout")
	}
	if !report.Completed || !report.Passed || report.FailureCode != "" {
		issues = append(issues, "session_not_passed")
	}
	if report.StopReason != "duration_elapsed" && report.StopReason != "canceled" {
		issues = append(issues, "invalid_passing_stop_reason")
	}
	if report.Supervisor.FinalState != "STOPPED" || report.Supervisor.ActiveCount == 0 {
		issues = append(issues, "missing_authenticated_session")
	}
	if !report.CrashGuard.Started || !report.CrashGuard.CleanShutdown {
		issues = append(issues, "missing_crash_guard_evidence")
	}
	input := report.DestinationInput
	if input.Entries == 0 || input.ReturnArms == 0 || input.KeyDowns+input.KeyUps+input.ButtonDowns+input.ButtonUps+input.PointerMoves+input.Wheels == 0 {
		issues = append(issues, "missing_destination_evidence")
	}
	return issues
}

func comparePair(first, second lab.AgentSessionReport) []string {
	issues := make([]string, 0)
	if first.DeviceAlias == second.DeviceAlias {
		issues = append(issues, "duplicate_device_alias")
	}
	if !sameLocationPair(first.DeviceAlias, second.DeviceAlias) {
		issues = append(issues, "mixed_location_pair")
	}
	if first.Role == second.Role {
		issues = append(issues, "missing_role_pair")
	}
	if first.Edge == second.Edge {
		issues = append(issues, "non_opposite_edges")
	}
	if first.BuildRevision != second.BuildRevision {
		issues = append(issues, "mixed_build_revision")
	}
	if first.BuildModified || second.BuildModified {
		issues = append(issues, "modified_build")
	}
	if first.RequestedMillis != second.RequestedMillis {
		issues = append(issues, "mixed_duration")
	}
	if first.HeartbeatMillis != second.HeartbeatMillis {
		issues = append(issues, "mixed_heartbeat_interval")
	}
	if first.PeerTimeoutMillis != second.PeerTimeoutMillis {
		issues = append(issues, "mixed_peer_timeout")
	}
	return issues
}

func sameLocationPair(first, second string) bool {
	return (first == "H-A" && second == "H-B") || (first == "H-B" && second == "H-A") ||
		(first == "S-A" && second == "S-B") || (first == "S-B" && second == "S-A")
}
