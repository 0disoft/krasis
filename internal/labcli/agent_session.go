package labcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/buildinfo"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/evidence"
	"github.com/0disoft/krasis/internal/lab"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/transport"
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
	edge          control.ScreenEdge
	duration      time.Duration
	heartbeat     time.Duration
	peerTimeout   time.Duration
	rearmDistance int32
	output        string
	guardOutput   string
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
	_, guardPath, err := resolveAgentSessionOutputPaths(options.output, options.guardOutput)
	if err != nil {
		fmt.Fprintln(stderr, "invalid agent-session guard output path")
		return exitUsage
	}
	build := buildinfo.Read()
	report := lab.AgentSessionReport{
		SchemaVersion: lab.AgentSessionSchemaVersion, DeviceAlias: options.device,
		Role: options.role, Edge: string(options.edge), CollectedAt: time.Now().UTC(),
		BuildRevision: build.Revision, BuildModified: build.Modified,
		RequestedMillis: options.duration.Milliseconds(), HeartbeatMillis: options.heartbeat.Milliseconds(),
		PeerTimeoutMillis: options.peerTimeout.Milliseconds(),
	}
	var guard *crashguard.Process
	guardClosed := false
	finish := func(code int) int {
		if guard != nil && !guardClosed {
			guardClosed = true
			if err := guard.Close(); err != nil {
				report.CrashGuard.CleanShutdown = false
				report.Passed = false
				if report.FailureCode == "" {
					report.FailureCode = "crash_guard_shutdown_failed"
				}
				code = exitRuntime
			} else {
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
	diagnostics, err := platform.Diagnose(ctx)
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
	executable, err := os.Executable()
	if err != nil {
		_ = source.Close()
		report.FailureCode = "crash_guard_executable_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	guard, err = crashguard.Start(executable, options.device, guardPath)
	if err != nil {
		_ = source.Close()
		report.FailureCode = "crash_guard_start_failed"
		report.StopReason = "failure"
		return finish(exitRuntime)
	}
	report.CrashGuard.Started = true
	report.CrashGuard.ReportFile = filepath.Base(guardPath)
	fmt.Fprintf(stdout, "Crash guard armed: agent_pid=%d report=%s\n", os.Getpid(), options.guardOutput)
	input := newCountingInput(wininput.NewInputAdapter(), guard)
	observer := &agentSessionObserver{}
	runContext, cancel := context.WithTimeout(ctx, options.duration)
	defer cancel()
	go func() {
		select {
		case <-guard.Done():
			cancel()
		case <-runContext.Done():
		}
	}()
	sessionConfig := newAgentSessionConfig(options, diagnostics.Display.Left, diagnostics.Display.Width, observer)
	err = agent.RunSupervisor(runContext, agent.SupervisorConfig{
		Session: sessionConfig,
		Backoff: agent.DefaultBackoffPolicy(), Now: time.Now, Wait: agent.Wait, Observer: observer,
	}, agent.SupervisorDependencies{
		Source: source, Hook: wininput.NewSourceHook(), Input: input,
		Effects: agent.RuntimeEffects{}, Authorizer: agent.AuthenticatedPeerAuthorizer{}, RunSession: agent.RunSession,
	})
	report.Supervisor = observer.evidence()
	report.Liveness = observer.livenessEvidence()
	report.DestinationInput = input.evidence()
	select {
	case <-guard.Done():
		report.FailureCode = "crash_guard_stopped"
		report.StopReason = "failure"
		return finish(exitRuntime)
	default:
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
	case errors.Is(err, context.Canceled) && ctx.Err() != nil:
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

func resolveAgentSessionOutputPaths(report, guard string) (string, string, error) {
	reportPath, err := filepath.Abs(report)
	if err != nil {
		return "", "", err
	}
	guardPath, err := filepath.Abs(guard)
	if err != nil {
		return "", "", err
	}
	if strings.EqualFold(filepath.Clean(reportPath), filepath.Clean(guardPath)) ||
		!strings.EqualFold(filepath.Clean(filepath.Dir(reportPath)), filepath.Clean(filepath.Dir(guardPath))) {
		return "", "", errors.New("agent and guard outputs must be different files in the same directory")
	}
	return reportPath, guardPath, nil
}

func agentSessionEvidenceExit(report *lab.AgentSessionReport) int {
	if report.Supervisor.ActiveCount == 0 {
		report.FailureCode = "no_authenticated_session"
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
	input := report.DestinationInput
	if input.Entries == 0 || input.KeyDowns+input.KeyUps+input.ButtonDowns+input.ButtonUps+input.PointerMoves+input.Wheels == 0 {
		report.FailureCode = "no_destination_input_observed"
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
		"--guard-output": &parsed.guardOutput,
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
	if parsed.address == "" || parsed.ca == "" || parsed.cert == "" || parsed.key == "" || parsed.output == "" || parsed.guardOutput == "" || edgeValue == "" || durationValue == "" || heartbeatValue == "" || peerTimeoutValue == "" {
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
	return config
}

func writeAgentSessionUsage(output io.Writer) {
	fmt.Fprintln(output, "usage: krasis-lab agent-session listen|connect --device ALIAS --address ADDRESS [--server-name NAME] --ca CA.pem --cert CERT.pem --key KEY.pem --edge LEFT|RIGHT --duration DURATION --heartbeat-interval DURATION --peer-timeout DURATION --output REPORT.json --guard-output GUARD.json [--rearm-distance PIXELS] --allow-live-input")
}

type countingInput struct {
	delegate *wininput.InputAdapter
	guard    inputGuard
	mu       sync.Mutex
	value    lab.InputEvidence
}

type inputGuard interface {
	PreparePress(control.PressedToken) error
	Released(control.PressedToken) error
}

func newCountingInput(delegate *wininput.InputAdapter, guard inputGuard) *countingInput {
	return &countingInput{delegate: delegate, guard: guard}
}
func (input *countingInput) Press(ctx context.Context, token control.PressedToken) error {
	if err := input.guard.PreparePress(token); err != nil {
		return fmt.Errorf("prepare crash recovery: %w", err)
	}
	if err := input.delegate.Press(ctx, token); err != nil {
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
	if err := input.delegate.Release(ctx, token); err != nil {
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
		return err
	}
	input.mu.Lock()
	input.value.PointerMoves++
	input.mu.Unlock()
	return nil
}
func (input *countingInput) Wheel(ctx context.Context, x, y int32) error {
	if err := input.delegate.Wheel(ctx, x, y); err != nil {
		return err
	}
	input.mu.Lock()
	input.value.Wheels++
	input.mu.Unlock()
	return nil
}
func (input *countingInput) Enter(ctx context.Context, edge control.ScreenEdge) error {
	if err := input.delegate.Enter(ctx, edge); err != nil {
		return err
	}
	input.mu.Lock()
	input.value.Entries++
	input.mu.Unlock()
	return nil
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
	mu                  sync.Mutex
	value               lab.SupervisorEvidence
	liveness            lab.LivenessEvidence
	receiveSession      string
	lastReceiveObserved time.Time
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
	case "emergency_release":
		observer.value.EmergencyStops++
	}
}
func (observer *agentSessionObserver) evidence() lab.SupervisorEvidence {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return observer.value
}

func (observer *agentSessionObserver) livenessEvidence() lab.LivenessEvidence {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return observer.liveness
}
