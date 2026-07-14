//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/guardreport"
	"github.com/0disoft/krasis/internal/lab"
)

const smokeBuildRevision = "agent-evidence-smoke"

type evidenceFixture struct {
	agentPaths    map[string]string
	recoveryPaths map[string]string
	reports       map[string]lab.AgentSessionReport
}

func TestAgentEvidenceCLIContractSmoke(t *testing.T) {
	if lab.AgentSessionSchemaVersion != "krasis.lab.agent-session/v7" {
		t.Fatalf("agent session schema = %q", lab.AgentSessionSchemaVersion)
	}
	executable := buildLabExecutable(t)
	fixture := writeEvidenceFixture(t)

	home := runLabJSON(t, executable, 0,
		"agent-session-check", "--json", fixture.agentPaths["H-A"], fixture.agentPaths["H-B"],
	)
	requireJSONValue(t, home, "schemaVersion", "krasis.lab.agent-session-check/v4")
	requireJSONValue(t, home, "pairPassed", true)
	requireDestinationInput(t, home)
	requireAggregatePrivacy(t, home)

	partial := fixture.reports["H-A"]
	partial.DestinationInput.InjectionFailures.PartiallyApplied = 1
	partialPath := writeJSONFile(t, filepath.Dir(fixture.agentPaths["H-A"]), "H-A-partial.json", partial)
	partialResult := runLabJSON(t, executable, 1,
		"agent-session-check", "--json", partialPath, fixture.agentPaths["H-B"],
	)
	requireFileIssue(t, partialResult, "unsafe_destination_input_evidence")
	requireAggregatePrivacy(t, partialResult)

	unclassifiedRollback := fixture.reports["H-A"]
	unclassifiedRollback.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 1}
	unclassifiedPath := writeJSONFile(t, filepath.Dir(fixture.agentPaths["H-A"]), "H-A-unclassified-rollback.json", unclassifiedRollback)
	unclassifiedResult := runLabJSON(t, executable, 1,
		"agent-session-check", "--json", unclassifiedPath, fixture.agentPaths["H-B"],
	)
	requireFileIssue(t, unclassifiedResult, "invalid_cursor_rollback_evidence")
	requireAggregatePrivacy(t, unclassifiedResult)

	failedRollback := fixture.reports["H-A"]
	failedRollback.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 1, Failed: 1}
	failedPath := writeJSONFile(t, filepath.Dir(fixture.agentPaths["H-A"]), "H-A-failed-rollback.json", failedRollback)
	failedResult := runLabJSON(t, executable, 1,
		"agent-session-check", "--json", failedPath, fixture.agentPaths["H-B"],
	)
	requireFileIssue(t, failedResult, "unsafe_cursor_rollback_evidence")
	requireAggregatePrivacy(t, failedResult)

	phase0 := runLabJSON(t, executable, 0,
		"phase0-check", "--json",
		"--home", fixture.agentPaths["H-A"], "--home", fixture.agentPaths["H-B"],
		"--study", fixture.agentPaths["S-A"], "--study", fixture.agentPaths["S-B"],
		"--recovery", fixture.recoveryPaths["H-A"], "--recovery", fixture.recoveryPaths["H-B"],
		"--recovery", fixture.recoveryPaths["S-A"], "--recovery", fixture.recoveryPaths["S-B"],
	)
	requireJSONValue(t, phase0, "schemaVersion", "krasis.lab.phase0-check/v4")
	requireJSONValue(t, phase0, "passed", true)
	homeResult := requireJSONObject(t, phase0, "home")
	requireJSONValue(t, homeResult, "schemaVersion", "krasis.lab.agent-session-check/v4")
	requireDestinationInput(t, homeResult)
	requireAggregatePrivacy(t, phase0)
}

func buildLabExecutable(t *testing.T) string {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "krasis-lab.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "build", "-o", executable, ".")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Run(); err != nil {
		t.Fatalf("build krasis-lab executable: %v\n%s", err, output.String())
	}
	return executable
}

func writeEvidenceFixture(t *testing.T) evidenceFixture {
	t.Helper()
	directory := t.TempDir()
	fixture := evidenceFixture{
		agentPaths:    make(map[string]string, 4),
		recoveryPaths: make(map[string]string, 4),
		reports:       make(map[string]lab.AgentSessionReport, 4),
	}
	collectedAt := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	for _, device := range []struct {
		alias string
		role  string
		edge  string
	}{
		{alias: "H-A", role: "listen", edge: "RIGHT"},
		{alias: "H-B", role: "connect", edge: "LEFT"},
		{alias: "S-A", role: "listen", edge: "RIGHT"},
		{alias: "S-B", role: "connect", edge: "LEFT"},
	} {
		guardName := device.alias + "-guard.json"
		writeJSONFile(t, directory, guardName, guardreport.Report{
			SchemaVersion: guardreport.SchemaVersion,
			DeviceAlias:   device.alias,
			CollectedAt:   collectedAt,
			BuildRevision: smokeBuildRevision,
			Reason:        string(crashguard.ExitDisarm),
			Completed:     true,
			Passed:        true,
		})
		report := lab.AgentSessionReport{
			SchemaVersion:     lab.AgentSessionSchemaVersion,
			DeviceAlias:       device.alias,
			Role:              device.role,
			Edge:              device.edge,
			CollectedAt:       collectedAt,
			BuildRevision:     smokeBuildRevision,
			RequestedMillis:   120_000,
			HeartbeatMillis:   500,
			PeerTimeoutMillis: 3_000,
			Completed:         true,
			Passed:            true,
			StopReason:        "duration_elapsed",
			Lifecycle:         lab.LifecycleEvidence{MonitorStarted: true, CleanShutdown: true},
			CrashGuard:        lab.CrashGuardEvidence{Started: true, CleanShutdown: true, ReportFile: guardName},
			Supervisor:        lab.SupervisorEvidence{FinalState: "STOPPED", ActiveCount: 1},
			Liveness:          lab.LivenessEvidence{HeartbeatsSent: 4, HeartbeatsReceived: 4, ReceiveGapSamples: 3, MaxReceiveGapMillis: 600},
			DestinationInput:  lab.InputEvidence{Entries: 1, PointerMoves: 1, ReturnArms: 1},
		}
		if device.alias == "H-A" {
			report.DestinationInput.CursorRollbacks = lab.CursorRollbackEvidence{Attempted: 1, NotRestored: 1}
		}
		fixture.reports[device.alias] = report
		fixture.agentPaths[device.alias] = writeJSONFile(t, directory, device.alias+"-agent.json", report)
		fixture.recoveryPaths[device.alias] = writeJSONFile(t, directory, device.alias+"-recovery.json", guardreport.Report{
			SchemaVersion: guardreport.SchemaVersion,
			DeviceAlias:   device.alias,
			CollectedAt:   collectedAt,
			BuildRevision: smokeBuildRevision,
			Reason:        string(crashguard.ExitParentEOF),
			Attempted:     1,
			Released:      1,
			Completed:     true,
			Passed:        true,
		})
	}
	return fixture
}

func writeJSONFile(t *testing.T, directory, name string, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func runLabJSON(t *testing.T, executable string, expectedExit int, args ...string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("run krasis-lab %v: %v", args, err)
		}
		exitCode = exitError.ExitCode()
	}
	if exitCode != expectedExit {
		t.Fatalf("krasis-lab %v exit=%d want=%d stdout=%s stderr=%s", args, exitCode, expectedExit, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode krasis-lab %v output: %v\nstdout=%s\nstderr=%s", args, err, stdout.String(), stderr.String())
	}
	return result
}

func requireJSONValue(t *testing.T, object map[string]any, key string, want any) {
	t.Helper()
	if got, exists := object[key]; !exists || got != want {
		t.Fatalf("JSON %s=%v, want %v", key, got, want)
	}
}

func requireJSONObject(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := object[key].(map[string]any)
	if !ok {
		t.Fatalf("JSON %s is not an object: %T", key, object[key])
	}
	return value
}

func requireDestinationInput(t *testing.T, report map[string]any) {
	t.Helper()
	files, ok := report["files"].([]any)
	if !ok || len(files) == 0 {
		t.Fatalf("checker files missing: %T", report["files"])
	}
	file, ok := files[0].(map[string]any)
	if !ok {
		t.Fatalf("checker file is not an object: %T", files[0])
	}
	input := requireJSONObject(t, file, "destinationInput")
	failures := requireJSONObject(t, input, "injectionFailures")
	requireJSONValue(t, failures, "notApplied", float64(0))
	rollbacks := requireJSONObject(t, input, "cursorRollbacks")
	requireJSONValue(t, rollbacks, "attempted", float64(1))
	requireJSONValue(t, rollbacks, "notRestored", float64(1))
}

func requireFileIssue(t *testing.T, report map[string]any, want string) {
	t.Helper()
	files, ok := report["files"].([]any)
	if !ok || len(files) == 0 {
		t.Fatalf("checker files missing: %T", report["files"])
	}
	file, ok := files[0].(map[string]any)
	if !ok {
		t.Fatalf("checker file is not an object: %T", files[0])
	}
	issues, ok := file["issues"].([]any)
	if !ok {
		t.Fatalf("checker issues missing: %T", file["issues"])
	}
	for _, issue := range issues {
		if issue == want {
			return
		}
	}
	t.Fatalf("checker issues=%v, want %q", issues, want)
}

func requireAggregatePrivacy(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]struct{}{
		"code": {}, "keyCode": {}, "buttonCode": {}, "x": {}, "y": {},
		"deltaX": {}, "deltaY": {}, "sessionID": {}, "epoch": {}, "leaseID": {},
		"address": {}, "certificate": {}, "certificateSHA256": {}, "fingerprint": {},
	}
	var inspect func(any)
	inspect = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if _, found := forbidden[key]; found {
					t.Fatalf("checker output exposed forbidden field %q", key)
				}
				inspect(child)
			}
		case []any:
			for _, child := range typed {
				inspect(child)
			}
		}
	}
	inspect(value)
}
