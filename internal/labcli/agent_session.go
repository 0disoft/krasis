package labcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/buildinfo"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/evidence"
	"github.com/0disoft/krasis/internal/lab"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
	"github.com/0disoft/krasis/internal/wininput"
)

const defaultAgentRearmDistance int32 = 32

type agentSessionArgs struct {
	role          string
	device        string
	address       string
	serverName    string
	ca            string
	cert          string
	key           string
	trustStore    string
	edge          control.ScreenEdge
	duration      time.Duration
	heartbeat     time.Duration
	peerTimeout   time.Duration
	rearmDistance int32
	output        string
	guardOutput   string
	stopFile      string
	liveInput     bool
}

func runAgentSession(ctx context.Context, args []string, stdout, stderr io.Writer, platform lab.Platform) int {
	options, err := parseAgentSessionArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "invalid agent-session options: %v\n", err)
		writeAgentSessionUsage(stderr)
		return exitUsage
	}
	if runtime.GOOS != "windows" {
		fmt.Fprintln(stderr, "unsupported: agent-session requires Windows")
		return exitUnsupported
	}
	if err := evidence.ValidateJSONOutputPath(options.output); err != nil {
		fmt.Fprintf(stderr, "invalid agent-session output: %v\n", err)
		return exitUsage
	}
	if err := evidence.ValidateJSONOutputPath(options.guardOutput); err != nil {
		fmt.Fprintf(stderr, "invalid agent-session guard output: %v\n", err)
		return exitUsage
	}
	_, guardPath, stopPath, err := resolveAgentSessionPaths(options.output, options.guardOutput, options.stopFile)
	if err != nil {
		fmt.Fprintln(stderr, "invalid agent-session output or stop-file path")
		return exitUsage
	}
	sessionContext, closeStopMonitor, err := monitorAgentStopFile(ctx, stopPath)
	if err != nil {
		fmt.Fprintln(stderr, "invalid agent-session stop-file path")
		return exitUsage
	}
	defer closeStopMonitor()
	build := buildinfo.Read()
	report := lab.AgentSessionReport{
		SchemaVersion: lab.AgentSessionSchemaVersion, DeviceAlias: options.device,
		Role: options.role, Edge: string(options.edge), CollectedAt: time.Now().UTC(),
		BuildRevision: build.Revision, BuildModified: build.Modified,
		RequestedMillis: options.duration.Milliseconds(), HeartbeatMillis: options.heartbeat.Milliseconds(),
		PeerTimeoutMillis: options.peerTimeout.Milliseconds(),
	}
	var processOwner *agentSessionProcessOwner
	finish := func(code int) int {
		if processOwner != nil {
			closed := processOwner.Close()
			if event := processOwner.Event(); event != "" {
				report.Lifecycle.Event = string(event)
				report.Lifecycle.RearmRequired = true
				report.Completed = true
				report.Passed = false
				report.FailureCode = lifecycleFailureCode(event)
				report.StopReason = "lifecycle_transition"
				code = exitRuntime
			}
			report.Lifecycle.CleanShutdown = closed.LifecycleErr == nil
			switch {
			case closed.GuardErr != nil:
				report.CrashGuard.CleanShutdown = false
				report.Passed = false
				report.FailureCode = "crash_guard_shutdown_failed"
				report.StopReason = "failure"
				code = exitRuntime
			case closed.SourceErr != nil:
				report.Passed = false
				report.FailureCode = "connection_source_shutdown_failed"
				report.StopReason = "failure"
				code = exitRuntime
			case closed.LifecycleErr != nil:
				report.Passed = false
				report.FailureCode = "lifecycle_monitor_shutdown_failed"
				report.StopReason = "failure"
				code = exitRuntime
			}
			if report.CrashGuard.Started && closed.GuardErr == nil {
				report.CrashGuard.CleanShutdown = true
			}
		}
		if err := evidence.WriteJSONExclusive(options.output, report); err != nil {
			fmt.Fprintf(stderr, "output_error: %v\n", err)
			return exitRuntime
		}
		fmt.Fprintf(stdout, "Agent session report: %s\n", options.output)
		return code
	}
	processOwner, err = startAgentSessionProcessOwner(sessionContext)
	if err != nil {
		report.FailureCode = "lifecycle_monitor_start_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	report.Lifecycle.MonitorStarted = true
	diagnostics, err := platform.Diagnose(processOwner.Context())
	if err != nil {
		report.FailureCode = "diagnose_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	if diagnostics.Display.MonitorCount != 1 {
		report.FailureCode = "multiple_monitors_unsupported"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	identity, roots, err := peercredentials.Load(options.ca, options.cert, options.key)
	if err != nil {
		report.FailureCode = "credential_load_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	if err := peercredentials.ValidateIdentityValidThrough(identity, roots, time.Now().Add(options.duration)); err != nil {
		report.FailureCode = "credential_lifetime_insufficient"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	store, err := truststore.New(options.trustStore)
	if err != nil {
		report.FailureCode = "trust_store_load_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	document, err := store.List()
	if err != nil {
		report.FailureCode = "trust_store_load_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	authorizer, err := newReloadingTrustStoreAuthorizerFromDocument(store, document)
	if err != nil {
		report.FailureCode = "trust_policy_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	var source agent.ConnectionSource
	switch options.role {
	case "listen":
		tlsConfig, configErr := transport.NewServerTLSConfig(identity, roots)
		if configErr != nil {
			report.FailureCode = "tls_config_failed"
			report.StopReason = "failure"
			return finish(exitRuntime)
		}
		listener, listenErr := agent.NewListenSource(options.address, tlsConfig)
		if listenErr != nil {
			report.FailureCode = "listen_failed"
			report.StopReason = "failure"
			return finish(exitRuntime)
		}
		source = listener
		fmt.Fprintf(stdout, "Agent listener ready: %s\n", listener.Address())
	case "connect":
		tlsConfig, configErr := transport.NewClientTLSConfig(identity, roots, options.serverName)
		if configErr != nil {
			report.FailureCode = "tls_config_failed"
			report.StopReason = "failure"
			return finish(exitRuntime)
		}
		dialer, dialErr := agent.NewDialSource(options.address, tlsConfig, 5*time.Second)
		if dialErr != nil {
			report.FailureCode = "dial_config_failed"
			report.StopReason = "failure"
			return finish(exitRuntime)
		}
		source = dialer
	}
	if err := processOwner.SetSource(source); err != nil {
		_ = source.Close()
		report.FailureCode = "lifecycle_source_ownership_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	executable, err := os.Executable()
	if err != nil {
		report.FailureCode = "crash_guard_executable_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	guard, err := crashguard.Start(executable, options.device, guardPath)
	if err != nil {
		report.FailureCode = "crash_guard_start_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	report.CrashGuard.Started = true
	report.CrashGuard.ReportFile = filepath.Base(guardPath)
	if err := processOwner.SetCrashGuard(guard); err != nil {
		_ = guard.Close()
		report.FailureCode = "lifecycle_guard_ownership_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	fmt.Fprintf(stdout, "Crash guard armed: agent_pid=%d report=%s\n", os.Getpid(), options.guardOutput)
	inputAdapter, err := wininput.NewInputAdapterWithReturnRearmDistance(options.rearmDistance, diagnostics.Display.Width)
	if err != nil {
		report.FailureCode = "return_boundary_config_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	input := newCountingInput(inputAdapter, guard)
	observer := &agentSessionObserver{}
	runDeadline := time.Now().Add(options.duration)
	if err := peercredentials.ValidateIdentityValidThrough(identity, roots, runDeadline); err != nil {
		report.FailureCode = "credential_lifetime_insufficient"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	runContext, cancel := context.WithDeadline(processOwner.Context(), runDeadline)
	defer cancel()
	sessionConfig := newAgentSessionConfig(options, diagnostics.Display.Left, diagnostics.Display.Width, observer)
	err = agent.RunSupervisor(runContext, agent.SupervisorConfig{
		Session: sessionConfig,
		Backoff: agent.DefaultBackoffPolicy(), Now: time.Now, Wait: agent.Wait, Observer: observer,
	}, agent.SupervisorDependencies{
		Source: source, Hook: wininput.NewSourceHookObserved(observer), Input: input,
		Effects: agent.RuntimeEffects{}, Authorizer: authorizer, RunSession: agent.RunSession,
	})
	report.Supervisor = observer.evidence()
	report.Liveness = observer.livenessEvidence()
	report.SourceInput = observer.sourceInputEvidence()
	report.SourcePipeline = observer.sourcePipelineEvidence()
	report.ControlFlow = observer.controlFlowEvidence()
	report.DestinationPipeline = observer.destinationPipelineEvidence()
	report.DestinationInput = input.evidence()
	if errors.Is(context.Cause(processOwner.Context()), errCrashGuardStopped) {
		report.FailureCode = "crash_guard_stopped"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	if event := processOwner.Event(); event != "" {
		report.Completed = true
		report.FailureCode = lifecycleFailureCode(event)
		report.StopReason = "lifecycle_transition"
		return finish(exitRuntime)
	}
	if errors.Is(context.Cause(processOwner.Context()), agentlifecycle.ErrMonitorStopped) {
		report.FailureCode = "lifecycle_monitor_stopped"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	if cause := context.Cause(sessionContext); cause != nil &&
		!errors.Is(cause, errAgentStopRequested) &&
		!errors.Is(cause, context.Canceled) &&
		!errors.Is(cause, context.DeadlineExceeded) {
		report.FailureCode = "stop_file_monitor_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	switch {
	case errors.Is(err, wininput.ErrEmergencyRelease):
		report.Completed = true
		report.StopReason = "emergency_release"
		return finish(exitSuccess)
	case errors.Is(err, context.DeadlineExceeded):
		report.Completed = true
		report.StopReason = "duration_elapsed"
		return finish(agentSessionEvidenceExit(&report))
	case errors.Is(context.Cause(sessionContext), errAgentStopRequested):
		report.Completed = true
		report.StopReason = "canceled"
		return finish(agentSessionEvidenceExit(&report))
	case errors.Is(err, context.Canceled) && sessionContext.Err() != nil:
		report.Completed = true
		report.StopReason = "canceled"
		return finish(agentSessionEvidenceExit(&report))
	default:
		report.FailureCode = "agent_session_failed"
		report.StopReason = "failure"
		fmt.Fprintln(stderr, "runtime_error: agent session failed; inspect the redacted report")
		return finish(exitRuntime)
	}
}

func lifecycleFailureCode(event agentlifecycle.Event) string {
	return "lifecycle_" + string(event)
}

type trustDocumentSource interface {
	List() (truststore.Document, error)
}

type reloadingTrustStoreAuthorizer struct {
	source trustDocumentSource
}

func newReloadingTrustStoreAuthorizer(source trustDocumentSource) (*reloadingTrustStoreAuthorizer, error) {
	if source == nil {
		return nil, errors.New("trust document source is required")
	}
	document, err := source.List()
	if err != nil {
		return nil, fmt.Errorf("load trust policy: %w", err)
	}
	return newReloadingTrustStoreAuthorizerFromDocument(source, document)
}

func newReloadingTrustStoreAuthorizerFromDocument(source trustDocumentSource, document truststore.Document) (*reloadingTrustStoreAuthorizer, error) {
	if source == nil {
		return nil, errors.New("trust document source is required")
	}
	if _, err := newTrustStoreAuthorizer(document); err != nil {
		return nil, fmt.Errorf("build trust policy: %w", err)
	}
	return &reloadingTrustStoreAuthorizer{source: source}, nil
}

func (authorizer *reloadingTrustStoreAuthorizer) AuthorizeLease(
	ctx context.Context,
	peerContext transport.AuthenticatedPeerContext,
	sessionID,
	epoch string,
	observedAt time.Time,
) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if authorizer == nil || authorizer.source == nil {
		return "", false, errors.New("reloading trust store authorizer is not initialized")
	}
	document, err := authorizer.source.List()
	if err != nil {
		return "", false, fmt.Errorf("reload trust policy: %w", err)
	}
	current, err := newTrustStoreAuthorizer(document)
	if err != nil {
		return "", false, fmt.Errorf("build reloaded trust policy: %w", err)
	}
	return current.AuthorizeLease(ctx, peerContext, sessionID, epoch, observedAt)
}

func newTrustStoreAuthorizer(document truststore.Document) (*agent.TrustedPeerAuthorizer, error) {
	grants := make([]agent.TrustedPeerGrant, 0, len(document.Peers))
	for _, record := range document.Peers {
		grants = append(grants, agent.TrustedPeerGrant{
			Peer: transport.AuthenticatedPeerContext{
				CertificateSHA256: record.Fingerprint,
				Role:              record.Role,
			},
			AllowInboundControl: record.AllowInboundControl,
			Revoked:             record.RevokedAt != nil,
		})
	}
	return agent.NewTrustedPeerAuthorizer(grants)
}

func resolveAgentSessionOutputPaths(report, guard string) (string, string, error) {
	reportPath, guardPath, _, err := resolveAgentSessionPaths(report, guard, "")
	return reportPath, guardPath, err
}

func resolveAgentSessionPaths(report, guard, stop string) (string, string, string, error) {
	reportPath, err := filepath.Abs(report)
	if err != nil {
		return "", "", "", err
	}
	guardPath, err := filepath.Abs(guard)
	if err != nil {
		return "", "", "", err
	}
	if strings.EqualFold(filepath.Clean(reportPath), filepath.Clean(guardPath)) ||
		!strings.EqualFold(filepath.Clean(filepath.Dir(reportPath)), filepath.Clean(filepath.Dir(guardPath))) {
		return "", "", "", errors.New("agent and guard outputs must be different files in the same directory")
	}
	if stop == "" {
		return reportPath, guardPath, "", nil
	}
	stopPath, err := filepath.Abs(stop)
	if err != nil {
		return "", "", "", err
	}
	if !strings.EqualFold(filepath.Clean(filepath.Dir(reportPath)), filepath.Clean(filepath.Dir(stopPath))) ||
		strings.EqualFold(filepath.Clean(reportPath), filepath.Clean(stopPath)) ||
		strings.EqualFold(filepath.Clean(guardPath), filepath.Clean(stopPath)) {
		return "", "", "", errors.New("stop file must be a third path in the report directory")
	}
	if _, err := os.Lstat(stopPath); err == nil {
		return "", "", "", errors.New("stop file already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", "", err
	}
	return reportPath, guardPath, stopPath, nil
}

func agentSessionEvidenceExit(report *lab.AgentSessionReport) int {
	if report.Supervisor.ActiveCount == 0 {
		report.FailureCode = "no_authenticated_session"
		return exitRuntime
	}
	destination := report.DestinationInput
	if destination.InjectionFailures.Total() > 0 {
		report.FailureCode = "input_injection_failure_observed"
		return exitRuntime
	}
	classifiedRollbacks, classifiedRollbacksValid := destination.CursorRollbacks.CheckedClassified()
	if !classifiedRollbacksValid || classifiedRollbacks != destination.CursorRollbacks.Attempted {
		report.FailureCode = "invalid_cursor_rollback_evidence"
		return exitRuntime
	}
	if destination.CursorRollbacks.Failed > 0 {
		report.FailureCode = "cursor_rollback_failed"
		return exitRuntime
	}
	destinationActions, destinationActionsValid := destination.CheckedTotalActions()
	if !destinationActionsValid {
		report.FailureCode = "invalid_destination_input_evidence"
		return exitRuntime
	}
	sourceActions, sourceActionsValid := report.SourceInput.CheckedTotalActions()
	if !sourceActionsValid {
		report.FailureCode = "invalid_source_pipeline_evidence"
		return exitRuntime
	}
	if !report.DestinationPipeline.ValidAgainst(report.DestinationInput) {
		report.FailureCode = "invalid_destination_pipeline_evidence"
		return exitRuntime
	}
	if report.DestinationPipeline.Rejected > 0 {
		report.FailureCode = "destination_input_rejected"
		return exitRuntime
	}
	if !report.SourcePipeline.ValidAgainst(report.SourceInput) {
		report.FailureCode = "invalid_source_pipeline_evidence"
		return exitRuntime
	}
	if !report.ControlFlow.ValidAgainst(report.SourceInput, report.SourcePipeline.Wheel, report.DestinationInput) {
		report.FailureCode = "invalid_control_flow_evidence"
		return exitRuntime
	}
	if !report.Supervisor.FailureEvidenceValid() {
		report.FailureCode = "invalid_failure_evidence"
		return exitRuntime
	}
	if report.ControlFlow.StageFailureObserved() {
		report.FailureCode = "control_flow_failure_observed"
		return exitRuntime
	}
	if report.ControlFlow.BoundaryDropObserved() {
		report.FailureCode = "boundary_not_ready_drop_observed"
		return exitRuntime
	}
	if report.Liveness.HeartbeatsSent < 2 || report.Liveness.HeartbeatsReceived < 2 || report.Liveness.ReceiveGapSamples == 0 {
		report.FailureCode = "insufficient_liveness_evidence"
		return exitRuntime
	}
	if report.Liveness.MaxReceiveGapMillis <= 0 || report.Liveness.MaxReceiveGapMillis >= report.PeerTimeoutMillis {
		report.FailureCode = "unsafe_liveness_gap"
		return exitRuntime
	}
	if report.SourcePipeline.Wheel.RawWheelUnforwarded() {
		report.FailureCode = "raw_wheel_observed_without_forwarding"
		return exitRuntime
	}
	if report.SourcePipeline.Wheel.WindowWheelUnforwarded() {
		report.FailureCode = "window_wheel_observed_without_forwarding"
		return exitRuntime
	}
	sourceObserved := report.SourceInput.BoundaryIntents > 0 && sourceActions > 0
	destinationObserved := destination.Entries > 0 && destination.ReturnArms > 0 && destinationActions > 0
	if !sourceObserved && !destinationObserved {
		report.FailureCode = "no_input_activity_observed"
		return exitRuntime
	}
	report.Passed = true
	return exitSuccess
}

func parseAgentSessionArgs(args []string) (agentSessionArgs, error) {
	var parsed agentSessionArgs
	if len(args) == 0 || (args[0] != "listen" && args[0] != "connect") {
		return parsed, errors.New("listen or connect mode is required")
	}
	parsed.role = args[0]
	parsed.rearmDistance = defaultAgentRearmDistance
	values := map[string]*string{
		"--device": &parsed.device, "--address": &parsed.address, "--server-name": &parsed.serverName,
		"--ca": &parsed.ca, "--cert": &parsed.cert, "--key": &parsed.key, "--output": &parsed.output,
		"--trust-store":  &parsed.trustStore,
		"--guard-output": &parsed.guardOutput,
		"--stop-file":    &parsed.stopFile,
	}
	var edgeValue, durationValue, heartbeatValue, peerTimeoutValue, rearmValue string
	values["--edge"] = &edgeValue
	values["--duration"] = &durationValue
	values["--heartbeat-interval"] = &heartbeatValue
	values["--peer-timeout"] = &peerTimeoutValue
	values["--rearm-distance"] = &rearmValue
	for index := 1; index < len(args); index++ {
		argument := args[index]
		if argument == "--allow-live-input" {
			if parsed.liveInput {
				return agentSessionArgs{}, errors.New("--allow-live-input may be specified only once")
			}
			parsed.liveInput = true
			continue
		}
		name, value, inline := strings.Cut(argument, "=")
		target, known := values[name]
		if !known {
			return agentSessionArgs{}, fmt.Errorf("unknown option %q", argument)
		}
		if !inline {
			if index+1 >= len(args) {
				return agentSessionArgs{}, fmt.Errorf("%s requires a value", name)
			}
			index++
			value = args[index]
		}
		if value == "" || *target != "" {
			return agentSessionArgs{}, fmt.Errorf("%s requires one non-empty value", name)
		}
		*target = value
	}
	if !parsed.liveInput {
		return agentSessionArgs{}, errors.New("--allow-live-input acknowledgement is required")
	}
	if !lab.ValidProbeDeviceAlias(parsed.device) {
		return agentSessionArgs{}, fmt.Errorf("unsupported device alias %q", parsed.device)
	}
	if parsed.address == "" || parsed.ca == "" || parsed.cert == "" || parsed.key == "" || parsed.trustStore == "" || parsed.output == "" || parsed.guardOutput == "" || edgeValue == "" || durationValue == "" || heartbeatValue == "" || peerTimeoutValue == "" {
		return agentSessionArgs{}, errors.New("device, address, credentials, edge, duration, heartbeat interval, peer timeout, and output are required")
	}
	parsed.edge = control.ScreenEdge(strings.ToUpper(edgeValue))
	if parsed.edge != control.ScreenEdgeLeft && parsed.edge != control.ScreenEdgeRight {
		return agentSessionArgs{}, errors.New("--edge must be LEFT or RIGHT")
	}
	duration, err := parsePositiveDuration(durationValue)
	if err != nil {
		return agentSessionArgs{}, err
	}
	if err := validateEvidenceDuration("--duration", duration); err != nil {
		return agentSessionArgs{}, err
	}
	parsed.duration = duration
	heartbeat, err := parsePositiveDuration(heartbeatValue)
	if err != nil {
		return agentSessionArgs{}, fmt.Errorf("invalid --heartbeat-interval: %w", err)
	}
	if err := validateEvidenceDuration("--heartbeat-interval", heartbeat); err != nil {
		return agentSessionArgs{}, err
	}
	peerTimeout, err := parsePositiveDuration(peerTimeoutValue)
	if err != nil {
		return agentSessionArgs{}, fmt.Errorf("invalid --peer-timeout: %w", err)
	}
	if err := validateEvidenceDuration("--peer-timeout", peerTimeout); err != nil {
		return agentSessionArgs{}, err
	}
	policy := agent.LivenessPolicy{HeartbeatInterval: heartbeat, PeerTimeout: peerTimeout}
	if err := policy.Validate(); err != nil {
		return agentSessionArgs{}, fmt.Errorf("invalid liveness policy: %w", err)
	}
	parsed.heartbeat = heartbeat
	parsed.peerTimeout = peerTimeout
	if rearmValue != "" {
		value, err := strconv.ParseInt(rearmValue, 10, 32)
		if err != nil || value <= 0 {
			return agentSessionArgs{}, errors.New("--rearm-distance must be a positive int32")
		}
		parsed.rearmDistance = int32(value)
	}
	if parsed.role == "connect" && parsed.serverName == "" {
		return agentSessionArgs{}, errors.New("--server-name is required in connect mode")
	}
	if parsed.role == "listen" && parsed.serverName != "" {
		return agentSessionArgs{}, errors.New("--server-name is only valid in connect mode")
	}
	return parsed, nil
}

func validateEvidenceDuration(name string, value time.Duration) error {
	if value < time.Millisecond || value%time.Millisecond != 0 {
		return fmt.Errorf("%s must be a whole number of milliseconds", name)
	}
	return nil
}

func newAgentSessionConfig(options agentSessionArgs, screenLeft, screenWidth int32, observer peer.LivenessObserver) agent.Config {
	config := agent.DefaultConfig(control.BoundaryConfig{
		Edge: options.edge, ScreenLeft: screenLeft, ScreenWidth: screenWidth,
		ActivationGap: 0, RearmDistance: options.rearmDistance,
	})
	config.Liveness = &agent.LivenessPolicy{
		HeartbeatInterval: options.heartbeat,
		PeerTimeout:       options.peerTimeout,
	}
	config.LivenessObserver = observer
	if sourceObserver, ok := observer.(agent.SourceActivityObserver); ok {
		config.SourceObserver = sourceObserver
	}
	if pipelineObserver, ok := observer.(wininput.SourcePipelineObserver); ok {
		config.PipelineObserver = pipelineObserver
	}
	if controlObserver, ok := observer.(peer.ControlFlowObserver); ok {
		config.ControlObserver = controlObserver
	}
	if destinationObserver, ok := observer.(application.InputDispositionObserver); ok {
		config.DestinationObserver = destinationObserver
	}
	return config
}

func writeAgentSessionUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: krasis-lab agent-session listen|connect --device ALIAS --address ADDRESS [--server-name NAME] --ca CA.pem --cert CERT.pem --key KEY.pem --trust-store DIRECTORY --edge LEFT|RIGHT --duration DURATION --heartbeat-interval DURATION --peer-timeout DURATION --output REPORT.json --guard-output GUARD.json [--stop-file PATH] [--rearm-distance PIXELS] --allow-live-input")
}

type countingInput struct {
	delegate    agent.InputBoundaryPort
	guard       inputGuard
	operationMu sync.Mutex
	mu          sync.Mutex
	value       lab.InputEvidence
}

type inputGuard interface {
	PreparePress(control.PressedToken) (bool, error)
	CancelPreparedPress(control.PressedToken) error
	Released(control.PressedToken) error
}

func newCountingInput(delegate agent.InputBoundaryPort, guard inputGuard) *countingInput {
	return &countingInput{delegate: delegate, guard: guard}
}
func (input *countingInput) Press(ctx context.Context, token control.PressedToken) error {
	input.operationMu.Lock()
	defer input.operationMu.Unlock()
	prepared, err := input.guard.PreparePress(token)
	if err != nil {
		return fmt.Errorf("prepare crash recovery: %w", err)
	}
	if err := input.delegate.Press(ctx, token); err != nil {
		input.recordInjectionFailure(err)
		certainty, classified := application.InjectionCertaintyOf(err)
		if prepared && classified && certainty == application.InjectionNotApplied {
			if cancelErr := input.guard.CancelPreparedPress(token); cancelErr != nil {
				return errors.Join(err, fmt.Errorf("cancel unapplied crash recovery press: %w", cancelErr))
			}
		}
		return err
	}
	input.mu.Lock()
	if token.Kind == control.PressedKeyboardKey {
		input.value.KeyDowns++
	} else {
		input.value.ButtonDowns++
	}
	input.mu.Unlock()
	return nil
}
func (input *countingInput) Release(ctx context.Context, token control.PressedToken) error {
	input.operationMu.Lock()
	defer input.operationMu.Unlock()
	if err := input.delegate.Release(ctx, token); err != nil {
		input.recordInjectionFailure(err)
		return err
	}
	if err := input.guard.Released(token); err != nil {
		return fmt.Errorf("confirm crash recovery release: %w", err)
	}
	input.mu.Lock()
	if token.Kind == control.PressedKeyboardKey {
		input.value.KeyUps++
	} else {
		input.value.ButtonUps++
	}
	input.mu.Unlock()
	return nil
}
func (input *countingInput) Move(ctx context.Context, x, y int32) error {
	if err := input.delegate.Move(ctx, x, y); err != nil {
		input.recordInjectionFailure(err)
		return err
	}
	input.mu.Lock()
	input.value.PointerMoves++
	input.mu.Unlock()
	return nil
}
func (input *countingInput) Wheel(ctx context.Context, x, y int32) error {
	if err := input.delegate.Wheel(ctx, x, y); err != nil {
		input.recordInjectionFailure(err)
		return err
	}
	input.mu.Lock()
	input.value.Wheels++
	input.mu.Unlock()
	return nil
}
func (input *countingInput) PrepareEntry(ctx context.Context, edge control.ScreenEdge) error {
	return input.delegate.PrepareEntry(ctx, edge)
}
func (input *countingInput) CommitEntry() {
	input.delegate.CommitEntry()
	input.mu.Lock()
	input.value.Entries++
	input.mu.Unlock()
}
func (input *countingInput) RollbackEntry(ctx context.Context) (application.CursorRollbackOutcome, error) {
	input.mu.Lock()
	input.value.CursorRollbacks.Attempted++
	input.mu.Unlock()
	outcome, err := input.delegate.RollbackEntry(ctx)
	input.mu.Lock()
	defer input.mu.Unlock()
	if err != nil {
		input.value.CursorRollbacks.Failed++
		return "", err
	}
	if validationErr := outcome.Validate(); validationErr != nil {
		input.value.CursorRollbacks.Failed++
		return "", validationErr
	}
	switch outcome {
	case application.CursorRollbackRestored:
		input.value.CursorRollbacks.Restored++
	case application.CursorRollbackNotRestored:
		input.value.CursorRollbacks.NotRestored++
	}
	return outcome, nil
}

func (input *countingInput) recordInjectionFailure(err error) {
	input.mu.Lock()
	defer input.mu.Unlock()
	certainty, classified := application.InjectionCertaintyOf(err)
	if !classified {
		input.value.InjectionFailures.Unclassified++
		return
	}
	switch certainty {
	case application.InjectionNotApplied:
		input.value.InjectionFailures.NotApplied++
	case application.InjectionPartiallyApplied:
		input.value.InjectionFailures.PartiallyApplied++
	case application.InjectionOutcomeUnknown:
		input.value.InjectionFailures.OutcomeUnknown++
	default:
		input.value.InjectionFailures.Unclassified++
	}
}
func (input *countingInput) ArmReturn(intent control.ReturnIntent, edge control.ScreenEdge) error {
	if err := input.delegate.ArmReturn(intent, edge); err != nil {
		return err
	}
	input.mu.Lock()
	input.value.ReturnArms++
	input.mu.Unlock()
	return nil
}
func (input *countingInput) RearmReturn(intent control.ReturnIntent) {
	input.delegate.RearmReturn(intent)
}
func (input *countingInput) DisarmReturn(intent control.ReturnIntent) {
	input.delegate.DisarmReturn(intent)
}
func (input *countingInput) ReturnIntents() <-chan control.ReturnIntent {
	return input.delegate.ReturnIntents()
}
func (input *countingInput) evidence() lab.InputEvidence {
	input.mu.Lock()
	defer input.mu.Unlock()
	return input.value
}

type agentSessionObserver struct {
	mu                            sync.Mutex
	rawWheelObserved              atomic.Uint64
	activeHookMoves               atomic.Uint64
	activeHookButtons             atomic.Uint64
	sourceQueueEnqueued           atomic.Uint64
	sourceQueueDequeued           atomic.Uint64
	sourceQueueDiscardedStale     atomic.Uint64
	sourceQueueMaxDepth           atomic.Uint64
	sourceQueueResidenceSamples   atomic.Uint64
	sourceQueueMaxResidenceMicros atomic.Int64
	destinationReceived           atomic.Uint64
	destinationApplied            atomic.Uint64
	destinationRetiredDrained     atomic.Uint64
	destinationRejected           atomic.Uint64
	value                         lab.SupervisorEvidence
	liveness                      lab.LivenessEvidence
	sourceInput                   lab.SourceInputEvidence
	sourcePipeline                lab.SourcePipelineEvidence
	controlFlow                   lab.ControlFlowEvidence
	failureCounts                 map[lab.FailureDescriptorEvidence]uint64
	receiveSession                string
	lastReceiveObserved           time.Time
}

func (observer *agentSessionObserver) ObserveSourceAction(action control.InputAction) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	switch action.Kind {
	case control.InputKeyDown:
		observer.sourceInput.KeyDowns++
	case control.InputKeyUp:
		observer.sourceInput.KeyUps++
	case control.InputButtonDown:
		observer.sourceInput.ButtonDowns++
	case control.InputButtonUp:
		observer.sourceInput.ButtonUps++
	case control.InputPointerMove:
		observer.sourceInput.PointerMoves++
	case control.InputWheel:
		observer.sourceInput.Wheels++
		observer.sourcePipeline.Wheel.Sent++
	}
}

func (observer *agentSessionObserver) ObserveWheelHook() {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.HookObserved++
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelSynthesized() {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.SynthesizedObserved++
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelRaw() {
	observer.rawWheelObserved.Add(1)
}

func (observer *agentSessionObserver) ObserveWheelWindow() {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.WindowObserved++
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelCaptureActivated() {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.CaptureActivations++
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelCaptureDeactivated() {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.CaptureDeactivations++
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelForegroundFallback(result wininput.SourceForegroundFallbackResult) {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.ForegroundFallbackAttempted++
	switch result {
	case wininput.SourceForegroundFallbackSucceeded:
		observer.sourcePipeline.Wheel.ForegroundFallbackSucceeded++
	case wininput.SourceForegroundFallbackAttachFailed:
		observer.sourcePipeline.Wheel.ForegroundFallbackAttachFailed++
	case wininput.SourceForegroundFallbackDetachFailed:
		observer.sourcePipeline.Wheel.ForegroundFallbackDetachFailed++
	case wininput.SourceForegroundFallbackAPIRejected:
		observer.sourcePipeline.Wheel.ForegroundFallbackAPIRejected++
	case wininput.SourceForegroundFallbackVerificationLost:
		observer.sourcePipeline.Wheel.ForegroundFallbackVerificationLost++
	}
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelFocusRestoreFallback(result wininput.SourceFocusRestoreFallbackResult) {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.FocusRestoreFallbackAttempted++
	switch result {
	case wininput.SourceFocusRestoreFallbackSucceeded:
		observer.sourcePipeline.Wheel.FocusRestoreFallbackSucceeded++
	case wininput.SourceFocusRestoreFallbackAttachFailed:
		observer.sourcePipeline.Wheel.FocusRestoreFallbackAttachFailed++
	case wininput.SourceFocusRestoreFallbackDetachFailed:
		observer.sourcePipeline.Wheel.FocusRestoreFallbackDetachFailed++
	case wininput.SourceFocusRestoreFallbackAPIRejected:
		observer.sourcePipeline.Wheel.FocusRestoreFallbackAPIRejected++
	case wininput.SourceFocusRestoreFallbackVerificationLost:
		observer.sourcePipeline.Wheel.FocusRestoreFallbackVerificationLost++
	}
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelFocusRestore(result wininput.SourceFocusRestoreResult) {
	observer.mu.Lock()
	switch result {
	case wininput.SourceFocusRestoreSkipped:
		observer.sourcePipeline.Wheel.FocusRestoreSkipped++
	case wininput.SourceFocusRestoreSucceeded:
		observer.sourcePipeline.Wheel.FocusRestored++
	case wininput.SourceFocusRestoreAPIRejected:
		observer.sourcePipeline.Wheel.FocusRestoreFailed++
		observer.sourcePipeline.Wheel.FocusRestoreAPIRejected++
	case wininput.SourceFocusRestoreVerificationLost:
		observer.sourcePipeline.Wheel.FocusRestoreFailed++
		observer.sourcePipeline.Wheel.FocusRestoreVerificationLost++
	}
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelPostHideFocus(result wininput.SourcePostHideFocusResult) {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.PostHideFocusAttempted++
	switch result {
	case wininput.SourcePostHideFocusRecovered:
		observer.sourcePipeline.Wheel.PostHideFocusRecovered++
	case wininput.SourcePostHideFocusUnavailable:
		observer.sourcePipeline.Wheel.PostHideFocusUnavailable++
	case wininput.SourcePostHideFocusCaptureRetained:
		observer.sourcePipeline.Wheel.PostHideFocusCaptureRetained++
	}
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveActiveHookMouseMove() {
	observer.activeHookMoves.Add(1)
}

func (observer *agentSessionObserver) ObserveActiveHookMouseButton() {
	observer.activeHookButtons.Add(1)
}

func (observer *agentSessionObserver) ObserveWheelMapped() {
	observer.mu.Lock()
	observer.sourcePipeline.Wheel.Mapped++
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveWheelQueue(result wininput.SourceQueueOfferResult) {
	observer.mu.Lock()
	switch result {
	case wininput.SourceQueueAccepted:
		observer.sourcePipeline.Wheel.QueueAccepted++
	case wininput.SourceQueueRejectedInactive:
		observer.sourcePipeline.Wheel.QueueRejected++
		observer.sourcePipeline.Wheel.RejectedInactive++
	case wininput.SourceQueueRejectedInvalid:
		observer.sourcePipeline.Wheel.QueueRejected++
		observer.sourcePipeline.Wheel.RejectedInvalid++
	case wininput.SourceQueueRejectedOverflow:
		observer.sourcePipeline.Wheel.QueueRejected++
		observer.sourcePipeline.Wheel.RejectedOverflow++
	}
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveSourceQueueEnqueued(depth uint64) {
	observer.sourceQueueEnqueued.Add(1)
	updateAtomicUint64Max(&observer.sourceQueueMaxDepth, depth)
}

func (observer *agentSessionObserver) ObserveSourceQueueDequeued(residence time.Duration) {
	observer.sourceQueueDequeued.Add(1)
	observer.sourceQueueResidenceSamples.Add(1)
	micros := residence.Microseconds()
	updateAtomicInt64Max(&observer.sourceQueueMaxResidenceMicros, micros)
}

func (observer *agentSessionObserver) ObserveSourceQueueDiscardedStale() {
	observer.sourceQueueDiscardedStale.Add(1)
}

func (observer *agentSessionObserver) ObserveDestinationInputReceived() {
	observer.destinationReceived.Add(1)
}

func (observer *agentSessionObserver) ObserveDestinationInputDisposition(disposition application.InputDisposition) {
	switch disposition {
	case application.InputDispositionApplied:
		observer.destinationApplied.Add(1)
	case application.InputDispositionRetiredDrained:
		observer.destinationRetiredDrained.Add(1)
	case application.InputDispositionRejected:
		observer.destinationRejected.Add(1)
	}
}

func updateAtomicUint64Max(value *atomic.Uint64, candidate uint64) {
	for current := value.Load(); candidate > current; current = value.Load() {
		if value.CompareAndSwap(current, candidate) {
			return
		}
	}
}

func updateAtomicInt64Max(value *atomic.Int64, candidate int64) {
	for current := value.Load(); candidate > current; current = value.Load() {
		if value.CompareAndSwap(current, candidate) {
			return
		}
	}
}

func (observer *agentSessionObserver) ObserveBoundaryIntent(control.BoundaryIntent) {
	observer.mu.Lock()
	observer.sourceInput.BoundaryIntents++
	observer.controlFlow.Boundary.Queued++
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveControlFlow(event peer.ControlFlowEvent) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	switch event {
	case peer.ControlFlowBoundaryAdmitted:
		observer.controlFlow.Boundary.Admitted++
	case peer.ControlFlowBoundaryDroppedNotReady:
		observer.controlFlow.Boundary.DroppedNotReady++
	case peer.ControlFlowBoundaryBlockedLocalMenu:
		observer.controlFlow.Boundary.BlockedLocalMenu++
	case peer.ControlFlowBoundaryBlockedMouseCapture:
		observer.controlFlow.Boundary.BlockedLocalInteraction++
		observer.controlFlow.Boundary.BlockedMouseCapture++
	case peer.ControlFlowBoundaryBlockedPressedInput:
		observer.controlFlow.Boundary.BlockedLocalInteraction++
		observer.controlFlow.Boundary.BlockedPressedInput++
	case peer.ControlFlowBoundaryBlockedMoveSize:
		observer.controlFlow.Boundary.BlockedLocalInteraction++
		observer.controlFlow.Boundary.BlockedMoveSize++
	case peer.ControlFlowBoundaryAdmissionFailed:
		observer.controlFlow.Boundary.AdmissionFailed++
	case peer.ControlFlowRequestWriteCompleted:
		observer.controlFlow.Outbound.RequestWriteCompleted++
	case peer.ControlFlowRequestWriteOutcomeUnknown:
		observer.controlFlow.Outbound.RequestWriteOutcomeUnknown++
	case peer.ControlFlowRequestReceived:
		observer.controlFlow.Inbound.RequestReceived++
	case peer.ControlFlowReturnArmSucceeded:
		observer.controlFlow.Inbound.ReturnArmSucceeded++
	case peer.ControlFlowEntryPrepareAttempted:
		observer.controlFlow.Inbound.EntryPrepareAttempted++
	case peer.ControlFlowEntryPrepared:
		observer.controlFlow.Inbound.EntryPrepared++
	case peer.ControlFlowGrantWriteCompleted:
		observer.controlFlow.Inbound.GrantWriteCompleted++
	case peer.ControlFlowGrantWriteOutcomeUnknown:
		observer.controlFlow.Inbound.GrantWriteOutcomeUnknown++
	case peer.ControlFlowEntryCommitted:
		observer.controlFlow.Inbound.EntryCommitted++
	case peer.ControlFlowEntryRollbackAttempted:
		observer.controlFlow.Inbound.EntryRollbackAttempted++
	case peer.ControlFlowGrantReceived:
		observer.controlFlow.Outbound.GrantReceived++
	case peer.ControlFlowGrantApplySucceeded:
		observer.controlFlow.Outbound.GrantApplySucceeded++
	case peer.ControlFlowGrantApplyFailed:
		observer.controlFlow.Outbound.GrantApplyFailed++
	case peer.ControlFlowSourceSessionActivationSucceeded:
		observer.controlFlow.Outbound.SourceSessionActivationSucceeded++
	case peer.ControlFlowSourceSessionActivationFailed:
		observer.controlFlow.Outbound.SourceSessionActivationFailed++
	case peer.ControlFlowCaptureActivationAttempted:
		observer.controlFlow.Outbound.CaptureActivationAttempted++
	case peer.ControlFlowCaptureActivationSucceeded:
		observer.controlFlow.Outbound.CaptureActivationSucceeded++
	case peer.ControlFlowCaptureActivationFailed:
		observer.controlFlow.Outbound.CaptureActivationFailed++
	case peer.ControlFlowCaptureActivationSetupFailed:
		observer.controlFlow.Outbound.CaptureActivationSetupFailed++
	case peer.ControlFlowCaptureActivationAPIRejected:
		observer.controlFlow.Outbound.CaptureActivationAPIRejected++
	case peer.ControlFlowCaptureActivationVerificationLost:
		observer.controlFlow.Outbound.CaptureActivationVerificationLost++
	case peer.ControlFlowCommitWriteCompleted:
		observer.controlFlow.Outbound.CommitWriteCompleted++
	case peer.ControlFlowCommitWriteOutcomeUnknown:
		observer.controlFlow.Outbound.CommitWriteOutcomeUnknown++
	case peer.ControlFlowAbortWriteCompleted:
		observer.controlFlow.Outbound.AbortWriteCompleted++
	case peer.ControlFlowAbortWriteOutcomeUnknown:
		observer.controlFlow.Outbound.AbortWriteOutcomeUnknown++
	case peer.ControlFlowCommitReceived:
		observer.controlFlow.Inbound.CommitReceived++
	case peer.ControlFlowAbortReceived:
		observer.controlFlow.Inbound.AbortReceived++
	case peer.ControlFlowRevokeWriteCompleted:
		observer.controlFlow.Inbound.RevokeWriteCompleted++
	case peer.ControlFlowRevokeWriteOutcomeUnknown:
		observer.controlFlow.Inbound.RevokeWriteOutcomeUnknown++
	case peer.ControlFlowRevokeReceived:
		observer.controlFlow.Outbound.RevokeReceived++
	case peer.ControlFlowLocalConnectionInterrupt:
		observer.controlFlow.Transport.LocalConnectionInterrupts++
	case peer.ControlFlowReadAfterLocalInterrupt:
		observer.controlFlow.Transport.ReadAfterLocalInterrupt++
	}
}

func (observer *agentSessionObserver) ObserveHeartbeatSent(_ string, _ time.Time) {
	observer.mu.Lock()
	observer.liveness.HeartbeatsSent++
	observer.mu.Unlock()
}

func (observer *agentSessionObserver) ObserveHeartbeatReceived(sessionID string, observedAt time.Time) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	observer.liveness.HeartbeatsReceived++
	if sessionID != observer.receiveSession {
		observer.receiveSession = sessionID
		observer.lastReceiveObserved = observedAt
		return
	}
	if !observer.lastReceiveObserved.IsZero() {
		gap := observedAt.Sub(observer.lastReceiveObserved)
		if gap >= 0 {
			observer.liveness.ReceiveGapSamples++
			if millis := gap.Milliseconds(); millis > observer.liveness.MaxReceiveGapMillis {
				observer.liveness.MaxReceiveGapMillis = millis
			}
		}
	}
	observer.lastReceiveObserved = observedAt
}

func (observer *agentSessionObserver) ObserveSupervisor(snapshot agent.SupervisorSnapshot) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	observer.value.FinalState = string(snapshot.State)
	if snapshot.State == agent.SupervisorActive {
		observer.value.ActiveCount++
	}
	switch snapshot.FailureKind {
	case "acquire":
		observer.value.AcquireFailures++
	case "session":
		observer.value.SessionFailures++
		descriptor := lab.FailureDescriptorEvidence{
			Component: snapshot.FailureComponent,
			Phase:     snapshot.FailurePhase,
			Code:      snapshot.FailureCode,
		}
		if observer.value.FirstFailure.Component == "" {
			observer.value.FirstFailure = descriptor
		}
		if observer.failureCounts == nil {
			observer.failureCounts = make(map[lab.FailureDescriptorEvidence]uint64)
		}
		observer.failureCounts[descriptor]++
	case "emergency_release":
		observer.value.EmergencyStops++
	}
}
func (observer *agentSessionObserver) evidence() lab.SupervisorEvidence {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	evidence := observer.value
	evidence.FailureCounts = make([]lab.FailureCountEvidence, 0, len(observer.failureCounts))
	for descriptor, count := range observer.failureCounts {
		evidence.FailureCounts = append(evidence.FailureCounts, lab.FailureCountEvidence{
			FailureDescriptorEvidence: descriptor,
			Count:                     count,
		})
	}
	sort.Slice(evidence.FailureCounts, func(left, right int) bool {
		first := evidence.FailureCounts[left]
		second := evidence.FailureCounts[right]
		if first.Component != second.Component {
			return first.Component < second.Component
		}
		if first.Phase != second.Phase {
			return first.Phase < second.Phase
		}
		return first.Code < second.Code
	})
	return evidence
}

func (observer *agentSessionObserver) livenessEvidence() lab.LivenessEvidence {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return observer.liveness
}

func (observer *agentSessionObserver) sourceInputEvidence() lab.SourceInputEvidence {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return observer.sourceInput
}

func (observer *agentSessionObserver) sourcePipelineEvidence() lab.SourcePipelineEvidence {
	observer.mu.Lock()
	evidence := observer.sourcePipeline
	observer.mu.Unlock()
	evidence.Wheel.RawObserved = observer.rawWheelObserved.Load()
	evidence.Wheel.ActiveHookMoves = observer.activeHookMoves.Load()
	evidence.Wheel.ActiveHookButtons = observer.activeHookButtons.Load()
	evidence.Queue = lab.SourceQueueEvidence{
		Enqueued:           observer.sourceQueueEnqueued.Load(),
		Dequeued:           observer.sourceQueueDequeued.Load(),
		DiscardedStale:     observer.sourceQueueDiscardedStale.Load(),
		MaxDepth:           observer.sourceQueueMaxDepth.Load(),
		ResidenceSamples:   observer.sourceQueueResidenceSamples.Load(),
		MaxResidenceMicros: observer.sourceQueueMaxResidenceMicros.Load(),
	}
	return evidence
}

func (observer *agentSessionObserver) controlFlowEvidence() lab.ControlFlowEvidence {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return observer.controlFlow
}

func (observer *agentSessionObserver) destinationPipelineEvidence() lab.DestinationPipelineEvidence {
	return lab.DestinationPipelineEvidence{
		Received:       observer.destinationReceived.Load(),
		Applied:        observer.destinationApplied.Load(),
		RetiredDrained: observer.destinationRetiredDrained.Load(),
		Rejected:       observer.destinationRejected.Load(),
	}
}
