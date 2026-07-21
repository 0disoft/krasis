package labcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/lab"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
	"github.com/0disoft/krasis/internal/wininput"
)

type certaintyInput struct {
	pressErr       error
	releaseErr     error
	moveErr        error
	wheelErr       error
	rollbackResult application.CursorRollbackOutcome
	rollbackErr    error
	returns        chan control.ReturnIntent
}

func (input *certaintyInput) Press(context.Context, control.PressedToken) error {
	return input.pressErr
}
func (input *certaintyInput) Release(context.Context, control.PressedToken) error {
	return input.releaseErr
}
func (input *certaintyInput) Move(context.Context, int32, int32) error         { return input.moveErr }
func (input *certaintyInput) Wheel(context.Context, int32, int32) error        { return input.wheelErr }
func (*certaintyInput) PrepareEntry(context.Context, control.ScreenEdge) error { return nil }
func (*certaintyInput) CommitEntry()                                           {}
func (input *certaintyInput) RollbackEntry(context.Context) (application.CursorRollbackOutcome, error) {
	if input.rollbackResult == "" && input.rollbackErr == nil {
		return application.CursorRollbackNotRestored, nil
	}
	return input.rollbackResult, input.rollbackErr
}
func (*certaintyInput) ArmReturn(control.ReturnIntent, control.ScreenEdge) error {
	return nil
}
func (*certaintyInput) RearmReturn(control.ReturnIntent)                 {}
func (*certaintyInput) DisarmReturn(control.ReturnIntent)                {}
func (input *certaintyInput) ReturnIntents() <-chan control.ReturnIntent { return input.returns }

type certaintyGuard struct {
	prepared bool
	canceled []control.PressedToken
	released []control.PressedToken
}

func (guard *certaintyGuard) PreparePress(control.PressedToken) (bool, error) {
	return guard.prepared, nil
}
func (guard *certaintyGuard) CancelPreparedPress(token control.PressedToken) error {
	guard.canceled = append(guard.canceled, token)
	return nil
}
func (guard *certaintyGuard) Released(token control.PressedToken) error {
	guard.released = append(guard.released, token)
	return nil
}

func TestCountingInputCancelsOnlyNewConfirmedUnappliedPress(t *testing.T) {
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	for _, test := range []struct {
		name      string
		prepared  bool
		certainty application.InjectionCertainty
		canceled  int
	}{
		{name: "new unapplied", prepared: true, certainty: application.InjectionNotApplied, canceled: 1},
		{name: "repeat unapplied", prepared: false, certainty: application.InjectionNotApplied, canceled: 0},
		{name: "new partial", prepared: true, certainty: application.InjectionPartiallyApplied, canceled: 0},
		{name: "new unknown", prepared: true, certainty: application.InjectionOutcomeUnknown, canceled: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			cause := errors.New("injection failed")
			delegate := &certaintyInput{pressErr: &application.InjectionError{
				Certainty: test.certainty, Requested: 1, Inserted: 0, Cause: cause,
			}}
			guard := &certaintyGuard{prepared: test.prepared}
			err := newCountingInput(delegate, guard).Press(context.Background(), token)
			if !errors.Is(err, cause) || len(guard.canceled) != test.canceled {
				t.Fatalf("Press() error=%v canceled=%v", err, guard.canceled)
			}
		})
	}
}

func TestCountingInputKeepsGuardRecordWhenReleaseFails(t *testing.T) {
	token := control.PressedToken{Kind: control.PressedMouseButton, Code: 1}
	cause := errors.New("release failed")
	delegate := &certaintyInput{releaseErr: &application.InjectionError{
		Certainty: application.InjectionNotApplied, Requested: 1, Inserted: 0, Cause: cause,
	}}
	guard := &certaintyGuard{prepared: true}
	err := newCountingInput(delegate, guard).Release(context.Background(), token)
	if !errors.Is(err, cause) || len(guard.released) != 0 {
		t.Fatalf("Release() error=%v released=%v", err, guard.released)
	}
}

func TestCountingInputClassifiesInjectionFailures(t *testing.T) {
	cause := errors.New("injection failed")
	input := newCountingInput(&certaintyInput{
		pressErr:   &application.InjectionError{Certainty: application.InjectionNotApplied, Requested: 1, Cause: cause},
		releaseErr: &application.InjectionError{Certainty: application.InjectionPartiallyApplied, Requested: 1, Inserted: 1, Cause: cause},
		moveErr:    &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Requested: 1, Cause: cause},
		wheelErr:   cause,
	}, &certaintyGuard{})
	token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
	_ = input.Press(context.Background(), token)
	_ = input.Release(context.Background(), token)
	_ = input.Move(context.Background(), 1, 0)
	_ = input.Wheel(context.Background(), 0, 1)
	evidence := input.evidence().InjectionFailures
	if evidence.NotApplied != 1 || evidence.PartiallyApplied != 1 || evidence.OutcomeUnknown != 1 || evidence.Unclassified != 1 || evidence.Total() != 4 {
		t.Fatalf("injection failure evidence = %+v", evidence)
	}
}

func TestCountingInputClassifiesCursorRollbackOutcomes(t *testing.T) {
	for _, test := range []struct {
		name     string
		outcome  application.CursorRollbackOutcome
		err      error
		want     lab.CursorRollbackEvidence
		wantFail bool
	}{
		{name: "restored", outcome: application.CursorRollbackRestored, want: lab.CursorRollbackEvidence{Attempted: 1, Restored: 1}},
		{name: "not restored", outcome: application.CursorRollbackNotRestored, want: lab.CursorRollbackEvidence{Attempted: 1, NotRestored: 1}},
		{name: "failed", err: errors.New("rollback failed"), want: lab.CursorRollbackEvidence{Attempted: 1, Failed: 1}, wantFail: true},
		{name: "invalid outcome", outcome: "UNKNOWN", want: lab.CursorRollbackEvidence{Attempted: 1, Failed: 1}, wantFail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := newCountingInput(&certaintyInput{rollbackResult: test.outcome, rollbackErr: test.err}, &certaintyGuard{})
			_, err := input.RollbackEntry(context.Background())
			if (err != nil) != test.wantFail {
				t.Fatalf("RollbackEntry() error = %v", err)
			}
			if evidence := input.evidence().CursorRollbacks; evidence != test.want {
				t.Fatalf("cursor rollback evidence = %+v, want %+v", evidence, test.want)
			}
		})
	}
}

func TestCountingInputCountsOnlyCommittedCursorEntry(t *testing.T) {
	input := newCountingInput(&certaintyInput{}, &certaintyGuard{})
	if err := input.PrepareEntry(context.Background(), control.ScreenEdgeRight); err != nil {
		t.Fatal(err)
	}
	if evidence := input.evidence(); evidence.Entries != 0 {
		t.Fatalf("prepared entry evidence = %+v", evidence)
	}
	input.CommitEntry()
	if evidence := input.evidence(); evidence.Entries != 1 {
		t.Fatalf("committed entry evidence = %+v", evidence)
	}
	if _, err := input.RollbackEntry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if evidence := input.evidence(); evidence.Entries != 1 {
		t.Fatalf("rollback changed committed entry evidence = %+v", evidence)
	}
}

func TestTrustStoreAuthorizerMapsActiveAndRevokedRecords(t *testing.T) {
	now := time.Now().UTC()
	peer := transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("a", 64), Role: transport.AuthenticatedPeerConnector,
	}
	document := truststore.Document{
		SchemaVersion: truststore.SchemaVersion, Revision: 1, UpdatedAt: now,
		Peers: []truststore.PeerRecord{{
			Fingerprint: peer.CertificateSHA256, Role: peer.Role, Alias: "peer",
			AllowInboundControl: true, CreatedAt: now,
		}},
	}
	authorizer, err := newTrustStoreAuthorizer(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, accepted, err := authorizer.AuthorizeLease(context.Background(), peer, "session", "epoch", now); err != nil || !accepted {
		t.Fatalf("active authorization accepted=%t error=%v", accepted, err)
	}
	document.Peers[0].RevokedAt = &now
	authorizer, err = newTrustStoreAuthorizer(document)
	if err != nil {
		t.Fatal(err)
	}
	if _, accepted, err := authorizer.AuthorizeLease(context.Background(), peer, "session", "epoch", now); err != nil || accepted {
		t.Fatalf("revoked authorization accepted=%t error=%v", accepted, err)
	}
}

type mutableTrustDocumentSource struct {
	document truststore.Document
	err      error
}

func (source *mutableTrustDocumentSource) List() (truststore.Document, error) {
	if source.err != nil {
		return truststore.Document{}, source.err
	}
	return source.document, nil
}

func TestReloadingTrustStoreAuthorizerAppliesRevocationToNextLease(t *testing.T) {
	now := time.Now().UTC()
	peerContext := transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("b", 64), Role: transport.AuthenticatedPeerConnector,
	}
	source := &mutableTrustDocumentSource{document: truststore.Document{
		SchemaVersion: truststore.SchemaVersion, Revision: 1, UpdatedAt: now,
		Peers: []truststore.PeerRecord{{
			Fingerprint: peerContext.CertificateSHA256, Role: peerContext.Role, Alias: "peer",
			AllowInboundControl: true, CreatedAt: now,
		}},
	}}
	authorizer, err := newReloadingTrustStoreAuthorizer(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, accepted, err := authorizer.AuthorizeLease(context.Background(), peerContext, "session-1", "epoch-1", now); err != nil || !accepted {
		t.Fatalf("active authorization accepted=%t error=%v", accepted, err)
	}

	revokedAt := now.Add(time.Second)
	source.document.Revision++
	source.document.UpdatedAt = revokedAt
	source.document.Peers[0].RevokedAt = &revokedAt
	if _, accepted, err := authorizer.AuthorizeLease(context.Background(), peerContext, "session-2", "epoch-2", revokedAt); err != nil || accepted {
		t.Fatalf("revoked authorization accepted=%t error=%v", accepted, err)
	}
}

func TestReloadingTrustStoreAuthorizerFailsClosedWhenReloadFails(t *testing.T) {
	now := time.Now().UTC()
	peerContext := transport.AuthenticatedPeerContext{
		CertificateSHA256: strings.Repeat("c", 64), Role: transport.AuthenticatedPeerConnector,
	}
	source := &mutableTrustDocumentSource{document: truststore.Document{
		SchemaVersion: truststore.SchemaVersion, Revision: 1, UpdatedAt: now,
		Peers: []truststore.PeerRecord{{
			Fingerprint: peerContext.CertificateSHA256, Role: peerContext.Role, Alias: "peer",
			AllowInboundControl: true, CreatedAt: now,
		}},
	}}
	authorizer, err := newReloadingTrustStoreAuthorizer(source)
	if err != nil {
		t.Fatal(err)
	}
	source.err = errors.New("test reload failure")
	if _, accepted, err := authorizer.AuthorizeLease(context.Background(), peerContext, "session", "epoch", now); err == nil || accepted {
		t.Fatalf("reload failure accepted=%t error=%v", accepted, err)
	}
}

func TestLifecycleFailureCodeIsStable(t *testing.T) {
	if code := lifecycleFailureCode("session_lock"); code != "lifecycle_session_lock" {
		t.Fatalf("lifecycleFailureCode() = %q", code)
	}
}

func TestResolveAgentSessionOutputPathsRequiresSameDirectory(t *testing.T) {
	first := filepath.Join(t.TempDir(), "agent.json")
	second := filepath.Join(t.TempDir(), "guard.json")
	if _, _, err := resolveAgentSessionOutputPaths(first, second); err == nil {
		t.Fatal("resolveAgentSessionOutputPaths accepted different directories")
	}
	directory := t.TempDir()
	if _, _, err := resolveAgentSessionOutputPaths(filepath.Join(directory, "agent.json"), filepath.Join(directory, "guard.json")); err != nil {
		t.Fatalf("resolveAgentSessionOutputPaths() error = %v", err)
	}
}

func TestResolveAgentSessionPathsRequiresFreshStopFileInReportDirectory(t *testing.T) {
	directory := t.TempDir()
	report := filepath.Join(directory, "agent.json")
	guard := filepath.Join(directory, "guard.json")
	stop := filepath.Join(directory, ".stop")
	_, _, resolvedStop, err := resolveAgentSessionPaths(report, guard, stop)
	if err != nil || resolvedStop != stop {
		t.Fatalf("resolveAgentSessionPaths() stop=%q error=%v", resolvedStop, err)
	}
	if _, _, _, err := resolveAgentSessionPaths(report, guard, filepath.Join(t.TempDir(), ".stop")); err == nil {
		t.Fatal("resolveAgentSessionPaths accepted stop file in another directory")
	}
	if err := os.WriteFile(stop, []byte("stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := resolveAgentSessionPaths(report, guard, stop); err == nil {
		t.Fatal("resolveAgentSessionPaths accepted an existing stop file")
	}
}

func TestParseAgentSessionConnectArgs(t *testing.T) {
	options, err := parseAgentSessionArgs([]string{
		"connect", "--device", "H-B", "--address", "192.0.2.10:24876", "--server-name", "192.0.2.10",
		"--ca", "ca.pem", "--cert", "identity.pem", "--key", "identity-key.pem",
		"--trust-store", "trust",
		"--edge", "left", "--duration", "2m", "--heartbeat-interval", "500ms", "--peer-timeout", "3s",
		"--output", "report.json", "--guard-output", "guard.json", "--stop-file", ".stop", "--allow-live-input",
	})
	if err != nil {
		t.Fatalf("parseAgentSessionArgs() error = %v", err)
	}
	if options.edge != control.ScreenEdgeLeft || options.duration != 2*time.Minute || options.heartbeat != 500*time.Millisecond || options.peerTimeout != 3*time.Second || options.rearmDistance != defaultAgentRearmDistance || options.stopFile != ".stop" {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseAgentSessionRequiresValidExplicitLivenessPolicy(t *testing.T) {
	base := []string{
		"listen", "--device", "H-A", "--address", ":24876", "--ca", "ca.pem",
		"--cert", "identity.pem", "--key", "identity-key.pem", "--edge", "RIGHT",
		"--trust-store", "trust",
		"--duration", "1m", "--output", "report.json", "--guard-output", "guard.json", "--allow-live-input",
	}
	if _, err := parseAgentSessionArgs(base); err == nil {
		t.Fatal("parseAgentSessionArgs accepted missing liveness policy")
	}
	inverted := append([]string{}, base...)
	inverted = append(inverted, "--heartbeat-interval", "3s", "--peer-timeout", "1s")
	if _, err := parseAgentSessionArgs(inverted); err == nil {
		t.Fatal("parseAgentSessionArgs accepted inverted liveness policy")
	}
}

func TestParseAgentSessionRejectsDurationThatCannotRoundTripThroughEvidence(t *testing.T) {
	for _, argument := range []string{"--duration", "--heartbeat-interval", "--peer-timeout"} {
		args := []string{
			"listen", "--device", "H-A", "--address", ":24876", "--ca", "ca.pem",
			"--cert", "identity.pem", "--key", "identity-key.pem", "--edge", "RIGHT",
			"--trust-store", "trust",
			"--duration", "1m", "--heartbeat-interval", "500ms", "--peer-timeout", "3s",
			"--output", "report.json", "--guard-output", "guard.json", "--allow-live-input",
		}
		for index := range args {
			if args[index] == argument {
				args[index+1] = "1500us"
				break
			}
		}
		if _, err := parseAgentSessionArgs(args); err == nil {
			t.Fatalf("parseAgentSessionArgs accepted non-millisecond %s", argument)
		}
	}
}

func TestNewAgentSessionConfigWiresExplicitLivenessPolicy(t *testing.T) {
	options := agentSessionArgs{
		edge: control.ScreenEdgeRight, rearmDistance: 48,
		heartbeat: 750 * time.Millisecond, peerTimeout: 4 * time.Second,
	}
	observer := &agentSessionObserver{}
	config := newAgentSessionConfig(options, -100, 200, observer)
	if config.Liveness == nil || config.Liveness.HeartbeatInterval != options.heartbeat || config.Liveness.PeerTimeout != options.peerTimeout {
		t.Fatalf("liveness config = %+v", config.Liveness)
	}
	if config.LivenessObserver != observer {
		t.Fatal("liveness observer was not wired")
	}
	if config.SourceObserver != observer {
		t.Fatal("source observer was not wired")
	}
	if config.PipelineObserver != observer {
		t.Fatal("pipeline observer was not wired")
	}
	if config.Boundary.Edge != options.edge || config.Boundary.ScreenLeft != -100 || config.Boundary.ScreenWidth != 200 || config.Boundary.RearmDistance != 48 {
		t.Fatalf("boundary config = %+v", config.Boundary)
	}
}

func TestParseAgentSessionRequiresLiveInputAcknowledgement(t *testing.T) {
	_, err := parseAgentSessionArgs([]string{
		"listen", "--device", "H-A", "--address", ":24876", "--ca", "ca.pem",
		"--cert", "identity.pem", "--key", "identity-key.pem", "--edge", "RIGHT",
		"--trust-store", "trust",
		"--duration", "1m", "--heartbeat-interval", "500ms", "--peer-timeout", "3s",
		"--output", "report.json", "--guard-output", "guard.json",
	})
	if err == nil {
		t.Fatal("parseAgentSessionArgs() accepted missing acknowledgement")
	}
}

func TestAgentSessionEvidenceRequiresConnectionAndLocalInputActivity(t *testing.T) {
	report := lab.AgentSessionReport{}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "no_authenticated_session" {
		t.Fatalf("no-session evidence = code %d report %+v", code, report)
	}
	report = lab.AgentSessionReport{Supervisor: lab.SupervisorEvidence{ActiveCount: 1}}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "insufficient_liveness_evidence" {
		t.Fatalf("no-liveness evidence = code %d report %+v", code, report)
	}
	report = lab.AgentSessionReport{
		PeerTimeoutMillis: 3_000,
		Supervisor:        lab.SupervisorEvidence{ActiveCount: 1},
		Liveness:          lab.LivenessEvidence{HeartbeatsSent: 2, HeartbeatsReceived: 2, ReceiveGapSamples: 1, MaxReceiveGapMillis: 500},
	}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "no_input_activity_observed" {
		t.Fatalf("no-input evidence = code %d report %+v", code, report)
	}
	report.SourceInput = lab.SourceInputEvidence{BoundaryIntents: 1, PointerMoves: 8}
	report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1,
	}
	report.SourcePipeline.Queue = lab.SourceQueueEvidence{
		Enqueued: 8, Dequeued: 8, MaxDepth: 1, ResidenceSamples: 8, MaxResidenceMicros: 1,
	}
	report.ControlFlow.Boundary = lab.BoundaryFlowEvidence{Queued: 1, Admitted: 1}
	report.ControlFlow.Outbound = lab.OutboundFlowEvidence{
		RequestWriteCompleted: 1, GrantReceived: 1, GrantApplySucceeded: 1,
		SourceSessionActivationSucceeded: 1, CaptureActivationAttempted: 1, CaptureActivationSucceeded: 1,
		CommitWriteCompleted: 1,
	}
	if code := agentSessionEvidenceExit(&report); code != exitSuccess || !report.Passed {
		t.Fatalf("source-only evidence = code %d report %+v", code, report)
	}
	report.Passed = false
	report.SourcePipeline.Wheel.RawObserved = 1
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "raw_wheel_observed_without_forwarding" {
		t.Fatalf("unforwarded raw wheel evidence = code %d report %+v", code, report)
	}
	report.SourcePipeline.Wheel.RawObserved = 0
	report.Passed = false
	report.SourcePipeline.Wheel.Sent = 1
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "invalid_source_pipeline_evidence" {
		t.Fatalf("invalid pipeline evidence = code %d report %+v", code, report)
	}
	report = lab.AgentSessionReport{
		PeerTimeoutMillis:   3_000,
		Supervisor:          lab.SupervisorEvidence{ActiveCount: 1},
		Liveness:            lab.LivenessEvidence{HeartbeatsSent: 2, HeartbeatsReceived: 2, ReceiveGapSamples: 1, MaxReceiveGapMillis: 500},
		DestinationInput:    lab.InputEvidence{Entries: 1, PointerMoves: 1},
		DestinationPipeline: lab.DestinationPipelineEvidence{Received: 1, Applied: 1},
		ControlFlow: lab.ControlFlowEvidence{Inbound: lab.InboundFlowEvidence{
			RequestReceived: 1, ReturnArmSucceeded: 0, EntryPrepareAttempted: 1, EntryPrepared: 1,
			GrantWriteCompleted: 1, CommitReceived: 1, EntryCommitted: 1,
		}},
	}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "no_input_activity_observed" {
		t.Fatalf("unarmed destination evidence = code %d report %+v", code, report)
	}
	report.FailureCode = ""
	report.DestinationInput.ReturnArms = 1
	report.ControlFlow.Inbound.ReturnArmSucceeded = 1
	if code := agentSessionEvidenceExit(&report); code != exitSuccess || !report.Passed {
		t.Fatalf("passing destination evidence = code %d report %+v", code, report)
	}
	report.Passed = false
	report.DestinationInput.InjectionFailures.OutcomeUnknown = 1
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "input_injection_failure_observed" {
		t.Fatalf("injection failure evidence = code %d report %+v", code, report)
	}
	report.DestinationInput.InjectionFailures = lab.InjectionFailureEvidence{}
	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 1, Failed: 1}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "cursor_rollback_failed" {
		t.Fatalf("rollback failure evidence = code %d report %+v", code, report)
	}
	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 1}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "invalid_cursor_rollback_evidence" {
		t.Fatalf("invalid rollback evidence = code %d report %+v", code, report)
	}
	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 1, Restored: 1}
	if code := agentSessionEvidenceExit(&report); code != exitSuccess || !report.Passed {
		t.Fatalf("safe rollback evidence = code %d report %+v", code, report)
	}
	report.Passed = false
	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{
		Attempted: ^uint64(0), Restored: ^uint64(0), NotRestored: 1,
	}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "invalid_cursor_rollback_evidence" {
		t.Fatalf("overflowing rollback evidence = code %d report %+v", code, report)
	}
	report.FailureCode = ""
	report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{}
	report.DestinationInput.PointerMoves = ^uint64(0)
	report.DestinationInput.Wheels = 1
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "invalid_destination_input_evidence" {
		t.Fatalf("overflowing destination evidence = code %d report %+v", code, report)
	}
	report.DestinationInput.PointerMoves = 1
	report.DestinationInput.Wheels = 0
	report.Passed = false
	report.Liveness.MaxReceiveGapMillis = report.PeerTimeoutMillis
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "unsafe_liveness_gap" {
		t.Fatalf("unsafe-gap evidence = code %d report %+v", code, report)
	}
}

func TestAgentSessionEvidenceClassifiesControlFlowFailures(t *testing.T) {
	base := lab.AgentSessionReport{
		PeerTimeoutMillis: 3_000,
		Supervisor:        lab.SupervisorEvidence{ActiveCount: 1},
		Liveness:          lab.LivenessEvidence{HeartbeatsSent: 2, HeartbeatsReceived: 2, ReceiveGapSamples: 1, MaxReceiveGapMillis: 500},
	}

	report := base
	report.SourceInput = lab.SourceInputEvidence{BoundaryIntents: 2, PointerMoves: 1}
	report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1,
	}
	report.SourcePipeline.Queue = lab.SourceQueueEvidence{
		Enqueued: 1, Dequeued: 1, MaxDepth: 1, ResidenceSamples: 1, MaxResidenceMicros: 1,
	}
	report.ControlFlow.Boundary = lab.BoundaryFlowEvidence{Queued: 2, Admitted: 1, DroppedNotReady: 1}
	report.ControlFlow.Outbound = lab.OutboundFlowEvidence{
		RequestWriteCompleted: 1, GrantReceived: 1, GrantApplySucceeded: 1,
		SourceSessionActivationSucceeded: 1, CaptureActivationAttempted: 1, CaptureActivationSucceeded: 1,
		CommitWriteCompleted: 1,
	}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "boundary_not_ready_drop_observed" {
		t.Fatalf("boundary drop evidence = code %d report %+v", code, report)
	}

	report = base
	report.SourceInput.BoundaryIntents = 1
	report.ControlFlow.Boundary = lab.BoundaryFlowEvidence{Queued: 1, AdmissionFailed: 1}
	report.ControlFlow.Outbound.RequestWriteOutcomeUnknown = 1
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "control_flow_failure_observed" {
		t.Fatalf("stage failure evidence = code %d report %+v", code, report)
	}

	report = lab.AgentSessionReport{
		PeerTimeoutMillis: 3_000,
		Supervisor: lab.SupervisorEvidence{
			ActiveCount:     1,
			SessionFailures: 1,
			FirstFailure: lab.FailureDescriptorEvidence{
				Component: "source-capture", Phase: "capture-activate", Code: "activation_failed",
			},
			FailureCounts: []lab.FailureCountEvidence{{
				FailureDescriptorEvidence: lab.FailureDescriptorEvidence{
					Component: "source-capture", Phase: "capture-activate", Code: "activation_failed",
				},
				Count: 1,
			}},
		},
		SourceInput: lab.SourceInputEvidence{BoundaryIntents: 1},
		ControlFlow: lab.ControlFlowEvidence{
			Boundary: lab.BoundaryFlowEvidence{Queued: 1, Admitted: 1},
			Outbound: lab.OutboundFlowEvidence{
				RequestWriteCompleted: 1, GrantReceived: 1, GrantApplySucceeded: 1,
				SourceSessionActivationSucceeded: 1, CaptureActivationAttempted: 1,
				CaptureActivationFailed: 1, CaptureActivationSetupFailed: 1, AbortWriteCompleted: 1,
			},
		},
	}
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "control_flow_failure_observed" {
		t.Fatalf("stage failure masked by liveness = code %d report %+v", code, report)
	}

	report = base
	report.SourceInput = lab.SourceInputEvidence{BoundaryIntents: 1, PointerMoves: 1}
	report.SourcePipeline.Wheel = lab.WheelPipelineEvidence{
		CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1,
	}
	report.SourcePipeline.Queue = lab.SourceQueueEvidence{
		Enqueued: 1, Dequeued: 1, MaxDepth: 1, ResidenceSamples: 1, MaxResidenceMicros: 1,
	}
	report.ControlFlow.Boundary.Queued = 1
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "invalid_control_flow_evidence" {
		t.Fatalf("invalid control flow evidence = code %d report %+v", code, report)
	}

	report = base
	report.Supervisor.SessionFailures = 1
	if code := agentSessionEvidenceExit(&report); code != exitRuntime || report.FailureCode != "invalid_failure_evidence" {
		t.Fatalf("invalid failure evidence = code %d report %+v", code, report)
	}
}

func TestAgentSessionObserverAggregatesReceiveGapsWithoutCrossingSessions(t *testing.T) {
	observer := &agentSessionObserver{}
	observer.ObserveHeartbeatSent("session-1", time.Unix(100, 0))
	observer.ObserveHeartbeatSent("session-1", time.Unix(101, 0))
	observer.ObserveHeartbeatReceived("session-1", time.Unix(100, 0))
	observer.ObserveHeartbeatReceived("session-1", time.Unix(100, 500_000_000))
	observer.ObserveHeartbeatReceived("session-1", time.Unix(101, 250_000_000))
	observer.ObserveHeartbeatReceived("session-2", time.Unix(200, 0))
	observer.ObserveHeartbeatReceived("session-2", time.Unix(200, 600_000_000))
	evidence := observer.livenessEvidence()
	if evidence.HeartbeatsSent != 2 || evidence.HeartbeatsReceived != 5 || evidence.ReceiveGapSamples != 3 || evidence.MaxReceiveGapMillis != 750 {
		t.Fatalf("liveness evidence = %+v", evidence)
	}
}

func TestAgentSessionObserverAggregatesSourceInputWithoutPayloads(t *testing.T) {
	observer := &agentSessionObserver{}
	observer.ObserveBoundaryIntent(control.BoundaryIntent{})
	observer.ObserveSourceAction(control.InputAction{Kind: control.InputPointerMove, DeltaX: 7})
	observer.ObserveSourceAction(control.InputAction{Kind: control.InputWheel, DeltaY: -1})
	observer.ObserveSourceAction(control.InputAction{Kind: control.InputKeyDown, Code: 65})
	evidence := observer.sourceInputEvidence()
	if evidence.BoundaryIntents != 1 || evidence.PointerMoves != 1 || evidence.Wheels != 1 || evidence.KeyDowns != 1 || evidence.TotalActions() != 3 {
		t.Fatalf("source input evidence = %+v", evidence)
	}
	pipeline := observer.sourcePipelineEvidence().Wheel
	if pipeline.Sent != 1 {
		t.Fatalf("source pipeline = %+v", pipeline)
	}
}

func TestAgentSessionObserverAggregatesWheelPipelineAndFailureComponent(t *testing.T) {
	observer := &agentSessionObserver{}
	observer.ObserveControlFlow(peer.ControlFlowBoundaryBlockedLocalMenu)
	observer.ObserveWheelHook()
	observer.ObserveWheelSynthesized()
	observer.ObserveWheelRaw()
	observer.ObserveWheelWindow()
	observer.ObserveWheelCaptureActivated()
	observer.ObserveWheelCaptureDeactivated()
	observer.ObserveWheelForegroundFallback(wininput.SourceForegroundFallbackSucceeded)
	observer.ObserveWheelForegroundFallback(wininput.SourceForegroundFallbackAttachFailed)
	observer.ObserveWheelForegroundFallback(wininput.SourceForegroundFallbackDetachFailed)
	observer.ObserveWheelForegroundFallback(wininput.SourceForegroundFallbackAPIRejected)
	observer.ObserveWheelForegroundFallback(wininput.SourceForegroundFallbackVerificationLost)
	observer.ObserveWheelFocusRestoreFallback(wininput.SourceFocusRestoreFallbackSucceeded)
	observer.ObserveWheelFocusRestoreFallback(wininput.SourceFocusRestoreFallbackAttachFailed)
	observer.ObserveWheelFocusRestoreFallback(wininput.SourceFocusRestoreFallbackDetachFailed)
	observer.ObserveWheelFocusRestoreFallback(wininput.SourceFocusRestoreFallbackAPIRejected)
	observer.ObserveWheelFocusRestoreFallback(wininput.SourceFocusRestoreFallbackVerificationLost)
	observer.ObserveWheelFocusRestore(wininput.SourceFocusRestoreSucceeded)
	observer.ObserveWheelFocusRestore(wininput.SourceFocusRestoreAPIRejected)
	observer.ObserveWheelFocusRestore(wininput.SourceFocusRestoreVerificationLost)
	observer.ObserveWheelPostHideFocus(wininput.SourcePostHideFocusRecovered)
	observer.ObserveWheelPostHideFocus(wininput.SourcePostHideFocusUnavailable)
	observer.ObserveWheelPostHideFocus(wininput.SourcePostHideFocusCaptureRetained)
	observer.ObserveActiveHookMouseMove()
	observer.ObserveActiveHookMouseButton()
	observer.ObserveWheelMapped()
	observer.ObserveWheelQueue(wininput.SourceQueueAccepted)
	observer.ObserveWheelQueue(wininput.SourceQueueRejectedInactive)
	observer.ObserveWheelQueue(wininput.SourceQueueRejectedInvalid)
	observer.ObserveWheelQueue(wininput.SourceQueueRejectedOverflow)
	observer.ObserveSupervisor(agent.SupervisorSnapshot{
		State: agent.SupervisorBackoff, FailureKind: "session", FailureComponent: "source-session",
		FailurePhase: "runtime", FailureCode: "component_failed",
	})
	observer.ObserveSupervisor(agent.SupervisorSnapshot{
		State: agent.SupervisorBackoff, FailureKind: "session", FailureComponent: "peer-session",
		FailurePhase: "runtime", FailureCode: "component_failed",
	})
	wheel := observer.sourcePipelineEvidence().Wheel
	if wheel.HookObserved != 1 || wheel.SynthesizedObserved != 1 || wheel.RawObserved != 1 || wheel.WindowObserved != 1 ||
		wheel.CaptureActivations != 1 || wheel.CaptureDeactivations != 1 || wheel.FocusRestored != 1 ||
		wheel.ForegroundFallbackAttempted != 5 || wheel.ForegroundFallbackSucceeded != 1 ||
		wheel.ForegroundFallbackAttachFailed != 1 || wheel.ForegroundFallbackDetachFailed != 1 ||
		wheel.ForegroundFallbackAPIRejected != 1 || wheel.ForegroundFallbackVerificationLost != 1 ||
		wheel.FocusRestoreFallbackAttempted != 5 || wheel.FocusRestoreFallbackSucceeded != 1 ||
		wheel.FocusRestoreFallbackAttachFailed != 1 || wheel.FocusRestoreFallbackDetachFailed != 1 ||
		wheel.FocusRestoreFallbackAPIRejected != 1 || wheel.FocusRestoreFallbackVerificationLost != 1 ||
		wheel.FocusRestoreFailed != 2 || wheel.FocusRestoreAPIRejected != 1 || wheel.FocusRestoreVerificationLost != 1 ||
		wheel.PostHideFocusAttempted != 3 || wheel.PostHideFocusRecovered != 1 || wheel.PostHideFocusUnavailable != 1 ||
		wheel.PostHideFocusCaptureRetained != 1 ||
		wheel.ActiveHookMoves != 1 || wheel.ActiveHookButtons != 1 || wheel.Mapped != 1 || wheel.QueueAccepted != 1 || wheel.QueueRejected != 3 ||
		wheel.RejectedInactive != 1 || wheel.RejectedInvalid != 1 || wheel.RejectedOverflow != 1 {
		t.Fatalf("wheel pipeline = %+v", wheel)
	}
	if evidence := observer.evidence(); evidence.FirstFailure.Component != "source-session" || evidence.SessionFailures != 2 || len(evidence.FailureCounts) != 2 {
		t.Fatalf("supervisor evidence = %+v", evidence)
	}
	if evidence := observer.controlFlowEvidence(); evidence.Boundary.BlockedLocalMenu != 1 {
		t.Fatalf("control flow evidence = %+v", evidence)
	}
}
