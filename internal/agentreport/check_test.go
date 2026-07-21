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
	if result.Files[0].DestinationInput != passingReport("H-A", "listen", "RIGHT").DestinationInput {
		t.Fatalf("destination input evidence = %+v", result.Files[0].DestinationInput)
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

func TestCheckRejectsLifecycleTransitionEvidence(t *testing.T) {
	directory := t.TempDir()
	report := passingReport("H-A", "listen", "RIGHT")
	report.Lifecycle.Event = "suspend"
	report.Lifecycle.RearmRequired = true
	path := writeReport(t, directory, "lifecycle.json", report)
	result := Check([]string{path})
	if result.Valid || !hasIssue(result.Files[0].Issues, "unsafe_lifecycle_evidence") {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"krasis.lab.agent-session/v21","surprise":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result := Check([]string{path})
	if result.Valid || !hasIssue(result.Files[0].Issues, "invalid_json") {
		t.Fatalf("Check() = %+v", result)
	}
}

func TestCheckRejectsMissingOrImpossibleLivenessEvidence(t *testing.T) {
	directory := t.TempDir()
	missingReport := passingReport("H-A", "listen", "RIGHT")
	missingReport.Liveness = lab.LivenessEvidence{}
	missing := writeReport(t, directory, "missing.json", missingReport)
	result := Check([]string{missing})
	if result.Valid || !hasIssue(result.Files[0].Issues, "insufficient_liveness_evidence") {
		t.Fatalf("Check(missing liveness) = %+v", result)
	}

	impossibleReport := passingReport("H-A", "listen", "RIGHT")
	impossibleReport.Liveness.ReceiveGapSamples = impossibleReport.Liveness.HeartbeatsReceived
	impossible := writeReport(t, directory, "impossible.json", impossibleReport)
	result = Check([]string{impossible})
	if result.Valid || !hasIssue(result.Files[0].Issues, "invalid_liveness_evidence") {
		t.Fatalf("Check(impossible liveness) = %+v", result)
	}

	unsafeReport := passingReport("H-A", "listen", "RIGHT")
	unsafeReport.Liveness.MaxReceiveGapMillis = unsafeReport.PeerTimeoutMillis
	unsafe := writeReport(t, directory, "unsafe.json", unsafeReport)
	result = Check([]string{unsafe})
	if result.Valid || !hasIssue(result.Files[0].Issues, "liveness_gap_reached_timeout") {
		t.Fatalf("Check(unsafe gap) = %+v", result)
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

func TestValidateRejectsUnsafeDestinationInputEvidence(t *testing.T) {
	report := passingReport("H-A", "listen", "RIGHT")
	report.DestinationInput.InjectionFailures.PartiallyApplied = 1
	issues := validate(report)
	if !hasIssue(issues, "unsafe_destination_input_evidence") {
		t.Fatalf("validate() issues = %v", issues)
	}

	report = passingReport("H-A", "listen", "RIGHT")
	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 2, Restored: 1}
	issues = validate(report)
	if !hasIssue(issues, "invalid_cursor_rollback_evidence") {
		t.Fatalf("validate() issues = %v", issues)
	}

	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 1, Failed: 1}
	issues = validate(report)
	if !hasIssue(issues, "unsafe_cursor_rollback_evidence") {
		t.Fatalf("validate() issues = %v", issues)
	}

	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 1, NotRestored: 1}
	if issues = validate(report); len(issues) != 0 {
		t.Fatalf("validate(safe no-restore) issues = %v", issues)
	}
}

func TestValidateRejectsOverflowingAggregateCounters(t *testing.T) {
	report := passingReport("H-A", "listen", "RIGHT")
	report.SourceInput = lab.SourceInputEvidence{BoundaryIntents: 1, KeyDowns: ^uint64(0), KeyUps: 1}
	report.DestinationInput.InjectionFailures = lab.InjectionFailureEvidence{NotApplied: ^uint64(0), PartiallyApplied: 1}
	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: ^uint64(0), Restored: ^uint64(0), Failed: 1}
	issues := validate(report)
	for _, want := range []string{
		"invalid_source_input_evidence",
		"invalid_source_pipeline_evidence",
		"invalid_injection_failure_evidence",
		"invalid_cursor_rollback_evidence",
	} {
		if !hasIssue(issues, want) {
			t.Fatalf("validate() issues = %v, want %q", issues, want)
		}
	}
}

func TestCheckRequiresComplementaryOneWayPairEvidence(t *testing.T) {
	directory := t.TempDir()
	firstReport := passingReport("H-A", "listen", "RIGHT")
	firstReport.SourceInput = lab.SourceInputEvidence{}
	secondReport := passingReport("H-B", "connect", "LEFT")
	secondReport.SourceInput = lab.SourceInputEvidence{}
	secondReport.SourcePipeline = lab.SourcePipelineEvidence{}
	secondReport.ControlFlow = lab.ControlFlowEvidence{}
	secondReport.DestinationInput = lab.InputEvidence{Entries: 1, PointerMoves: 1, ReturnArms: 1}
	secondReport.DestinationPipeline = lab.DestinationPipelineEvidence{Received: 1, Applied: 1}
	secondReport.ControlFlow.Inbound = lab.InboundFlowEvidence{
		RequestReceived: 1, ReturnArmSucceeded: 1, EntryPrepareAttempted: 1, EntryPrepared: 1,
		GrantWriteCompleted: 1, CommitReceived: 1, EntryCommitted: 1,
	}
	first := writeReport(t, directory, "H-A-destination.json", firstReport)
	second := writeReport(t, directory, "H-B-destination.json", secondReport)
	result := Check([]string{first, second})
	if !result.Valid || result.PairPassed || !hasIssue(result.Issues, "missing_pair_source_evidence") {
		t.Fatalf("destination-only pair = %+v", result)
	}

	firstReport = passingReport("H-A", "listen", "RIGHT")
	firstReport.DestinationInput = lab.InputEvidence{}
	firstReport.ControlFlow.Inbound = lab.InboundFlowEvidence{}
	firstReport.SourceInput = lab.SourceInputEvidence{BoundaryIntents: 1, PointerMoves: 1}
	firstReport.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1,
	}
	firstReport.SourcePipeline.Queue = lab.SourceQueueEvidence{
		Enqueued: 1, Dequeued: 1, MaxDepth: 1, ResidenceSamples: 1, MaxResidenceMicros: 1,
	}
	firstReport.DestinationPipeline = lab.DestinationPipelineEvidence{}
	firstReport.ControlFlow.Boundary = lab.BoundaryFlowEvidence{Queued: 1, Admitted: 1}
	firstReport.ControlFlow.Outbound = lab.OutboundFlowEvidence{
		RequestWriteCompleted: 1, GrantReceived: 1, GrantApplySucceeded: 1,
		SourceSessionActivationSucceeded: 1, CaptureActivationAttempted: 1, CaptureActivationSucceeded: 1,
		CommitWriteCompleted: 1,
	}
	secondReport = passingReport("H-B", "connect", "LEFT")
	secondReport.DestinationInput = lab.InputEvidence{}
	secondReport.DestinationPipeline = lab.DestinationPipelineEvidence{}
	first = writeReport(t, directory, "H-A-source.json", firstReport)
	second = writeReport(t, directory, "H-B-source.json", secondReport)
	result = Check([]string{first, second})
	if !result.Valid || result.PairPassed || !hasIssue(result.Issues, "missing_pair_destination_evidence") {
		t.Fatalf("source-only pair = %+v", result)
	}
}

func TestValidateAllowsIncompleteOppositeDirectionWhenLocalEvidenceIsComplete(t *testing.T) {
	destination := passingReport("H-A", "listen", "RIGHT")
	destination.SourceInput.BoundaryIntents = 1
	destination.ControlFlow.Boundary.Queued = 1
	if issues := validate(destination); len(issues) != 0 {
		t.Fatalf("complete destination with partial source issues = %v", issues)
	}

	source := passingReport("H-B", "connect", "LEFT")
	source.DestinationInput.Entries = 1
	source.ControlFlow.Inbound.EntryPrepareAttempted = 1
	source.ControlFlow.Inbound.EntryPrepared = 1
	source.ControlFlow.Inbound.GrantWriteCompleted = 1
	source.ControlFlow.Inbound.CommitReceived = 1
	source.ControlFlow.Inbound.EntryCommitted = 1
	if issues := validate(source); len(issues) != 0 {
		t.Fatalf("complete source with partial destination issues = %v", issues)
	}
}

func TestValidateAllowsSafeLocalMenuBlockAfterCompletedSourceInput(t *testing.T) {
	report := passingReport("H-B", "connect", "LEFT")
	report.SourceInput.BoundaryIntents = 2
	report.ControlFlow.Boundary = lab.BoundaryFlowEvidence{
		Queued: 2, Admitted: 1, BlockedLocalMenu: 1,
	}
	if issues := validate(report); len(issues) != 0 {
		t.Fatalf("local menu block issues = %v", issues)
	}
}

func TestValidateRejectsInvalidSourcePipelineEvidenceAndFailureComponent(t *testing.T) {
	report := passingReport("H-B", "connect", "LEFT")
	report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		HookObserved: 1, Mapped: 1, QueueAccepted: 1, Sent: 1,
	}
	issues := validate(report)
	if !hasIssue(issues, "invalid_source_pipeline_evidence") {
		t.Fatalf("pipeline issues = %v", issues)
	}

	report.SourceInput.Wheels = 1
	report.SourcePipeline.Queue = lab.SourceQueueEvidence{
		Enqueued: 2, Dequeued: 2, MaxDepth: 1, ResidenceSamples: 2, MaxResidenceMicros: 1,
	}
	report.SourcePipeline.Wheel.CaptureActivations = 1
	report.SourcePipeline.Wheel.CaptureDeactivations = 1
	report.SourcePipeline.Wheel.FocusRestoreSkipped = 1
	report.Supervisor.SessionFailures = 1
	report.Supervisor.FirstFailure = lab.FailureDescriptorEvidence{Component: "private-error", Phase: "runtime", Code: "component_failed"}
	report.Supervisor.FailureCounts = []lab.FailureCountEvidence{{
		FailureDescriptorEvidence: report.Supervisor.FirstFailure, Count: 1,
	}}
	issues = validate(report)
	if hasIssue(issues, "invalid_source_pipeline_evidence") || !hasIssue(issues, "invalid_failure_evidence") {
		t.Fatalf("component issues = %v", issues)
	}

	report.Supervisor.FirstFailure = lab.FailureDescriptorEvidence{Component: "liveness", Phase: "heartbeat", Code: "peer_timeout"}
	report.Supervisor.FailureCounts[0].FailureDescriptorEvidence = report.Supervisor.FirstFailure
	if issues = validate(report); len(issues) != 0 {
		t.Fatalf("valid pipeline issues = %v", issues)
	}
}

func TestValidateRejectsRawWheelObservedWithoutForwarding(t *testing.T) {
	report := passingReport("H-B", "connect", "LEFT")
	report.SourcePipeline.Wheel.RawObserved = 12
	issues := validate(report)
	if !hasIssue(issues, "raw_wheel_observed_without_forwarding") {
		t.Fatalf("raw wheel issues = %v", issues)
	}
}

func TestValidateRejectsWindowWheelWithoutForwardingAndUnsafeCaptureLifecycle(t *testing.T) {
	report := passingReport("H-B", "connect", "LEFT")
	report.SourcePipeline.Wheel.WindowObserved = 12
	issues := validate(report)
	if !hasIssue(issues, "window_wheel_observed_without_forwarding") {
		t.Fatalf("window wheel issues = %v", issues)
	}

	report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		CaptureActivations: 1,
	}
	issues = validate(report)
	if !hasIssue(issues, "invalid_source_pipeline_evidence") {
		t.Fatalf("capture lifecycle issues = %v", issues)
	}
}

func TestValidateRejectsUnclassifiedFocusRestoreFailure(t *testing.T) {
	report := passingReport("H-B", "connect", "LEFT")
	report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFailed: 1,
	}
	issues := validate(report)
	if !hasIssue(issues, "invalid_source_pipeline_evidence") {
		t.Fatalf("focus restore issues = %v", issues)
	}
}

func TestValidateAcceptsDegradedFocusRestoreAfterCaptureRelease(t *testing.T) {
	report := passingReport("H-B", "connect", "LEFT")
	report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		CaptureActivations: 1, CaptureDeactivations: 1,
		FocusRestoreFallbackAttempted: 1, FocusRestoreFallbackAPIRejected: 1,
		FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1,
		PostHideFocusAttempted: 1, PostHideFocusRecovered: 1,
	}
	if issues := validate(report); len(issues) != 0 {
		t.Fatalf("degraded focus restore issues = %v", issues)
	}
}

func TestValidateRejectsRetainedCaptureAfterFocusRestoreFailure(t *testing.T) {
	report := passingReport("H-B", "connect", "LEFT")
	report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		CaptureActivations: 1, CaptureDeactivations: 1,
		FocusRestoreFallbackAttempted: 1, FocusRestoreFallbackAPIRejected: 1,
		FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1,
		PostHideFocusAttempted: 1, PostHideFocusCaptureRetained: 1,
	}
	if issues := validate(report); !hasIssue(issues, "invalid_source_pipeline_evidence") {
		t.Fatalf("retained capture issues = %v", issues)
	}
}

func passingReport(alias, role, edge string) lab.AgentSessionReport {
	report := lab.AgentSessionReport{
		SchemaVersion: lab.AgentSessionSchemaVersion, DeviceAlias: alias, Role: role, Edge: edge,
		CollectedAt:   time.Date(2026, time.July, 11, 0, 0, 0, 0, time.UTC),
		BuildRevision: "abc123", RequestedMillis: 120_000, HeartbeatMillis: 500, PeerTimeoutMillis: 3_000,
		Completed: true, Passed: true, StopReason: "duration_elapsed",
		Lifecycle:  lab.LifecycleEvidence{MonitorStarted: true, CleanShutdown: true},
		Liveness:   lab.LivenessEvidence{HeartbeatsSent: 4, HeartbeatsReceived: 4, ReceiveGapSamples: 3, MaxReceiveGapMillis: 600},
		CrashGuard: lab.CrashGuardEvidence{Started: true, CleanShutdown: true, ReportFile: alias + "-guard.json"},
		Supervisor: lab.SupervisorEvidence{FinalState: "STOPPED", ActiveCount: 1},
	}
	if role == "listen" {
		report.DestinationInput = lab.InputEvidence{Entries: 1, PointerMoves: 1, ReturnArms: 1}
		report.DestinationPipeline = lab.DestinationPipelineEvidence{Received: 1, Applied: 1}
		report.ControlFlow.Inbound = lab.InboundFlowEvidence{
			RequestReceived: 1, ReturnArmSucceeded: 1, EntryPrepareAttempted: 1, EntryPrepared: 1,
			GrantWriteCompleted: 1, CommitReceived: 1, EntryCommitted: 1,
		}
	} else {
		report.SourceInput = lab.SourceInputEvidence{BoundaryIntents: 1, PointerMoves: 1}
		report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
			CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1,
		}
		report.SourcePipeline.Queue = lab.SourceQueueEvidence{
			Enqueued: 1, Dequeued: 1, MaxDepth: 1, ResidenceSamples: 1, MaxResidenceMicros: 1,
		}
		report.ControlFlow.Boundary = lab.BoundaryFlowEvidence{Queued: 1, Admitted: 1}
		report.ControlFlow.Outbound = lab.OutboundFlowEvidence{
			RequestWriteCompleted: 1, GrantReceived: 1, GrantApplySucceeded: 1,
			SourceSessionActivationSucceeded: 1, CaptureActivationAttempted: 1, CaptureActivationSucceeded: 1,
			CommitWriteCompleted: 1,
		}
	}
	return report
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
