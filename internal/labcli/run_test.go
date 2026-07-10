package labcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/lab"
)

type fakePlatform struct {
	diagnostics lab.Diagnostics
	diagnoseErr error
	capture     []lab.CaptureSnapshot
	captureErr  error
	selfTest    lab.SelfTestResult
	selfTestErr error
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("closed output")
}

func (f fakePlatform) Diagnose(context.Context) (lab.Diagnostics, error) {
	return f.diagnostics, f.diagnoseErr
}

func (f fakePlatform) Capture(_ context.Context, _ lab.CaptureOptions, report func(lab.CaptureSnapshot)) error {
	for _, snapshot := range f.capture {
		report(snapshot)
	}
	return f.captureErr
}

func (f fakePlatform) SelfTestInject(context.Context) (lab.SelfTestResult, error) {
	return f.selfTest, f.selfTestErr
}

func TestRunRequiresCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), nil, &stdout, &stderr, fakePlatform{})

	if exitCode != exitUsage {
		t.Fatalf("exit code = %d, want %d", exitCode, exitUsage)
	}
	if !strings.Contains(stderr.String(), "krasis-lab diagnose") {
		t.Fatalf("stderr does not contain usage: %q", stderr.String())
	}
}

func TestDiagnoseJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	platform := fakePlatform{diagnostics: lab.Diagnostics{
		SchemaVersion: lab.DiagnoseSchemaVersion,
		OS:            "windows",
		Architecture:  "amd64",
	}}

	exitCode := Run(context.Background(), []string{"diagnose", "--json"}, &stdout, &stderr, platform)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schemaVersion":"krasis.lab.diagnose/v1"`) {
		t.Fatalf("stdout does not contain schema version: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "keyCode") || strings.Contains(stdout.String(), "coordinates") {
		t.Fatalf("stdout contains forbidden raw input fields: %q", stdout.String())
	}
}

func TestCaptureReportsOnlyAggregateCounts(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	platform := fakePlatform{capture: []lab.CaptureSnapshot{{
		SchemaVersion: lab.CaptureSchemaVersion,
		ElapsedMillis: 25,
		Keyboard: lab.EventCounts{
			PhysicalDown: 1,
		},
	}}}

	exitCode := Run(context.Background(), []string{"capture", "--json"}, &stdout, &stderr, platform)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"physicalDown":1`) {
		t.Fatalf("stdout does not contain aggregate count: %q", stdout.String())
	}
	for _, forbidden := range []string{"vkCode", "scanCode", "x", "y", "key"} {
		if strings.Contains(stdout.String(), `"`+forbidden+`"`) {
			t.Fatalf("stdout contains forbidden field %q: %q", forbidden, stdout.String())
		}
	}
}

func TestUnsupportedExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	platform := fakePlatform{diagnoseErr: errors.Join(lab.ErrUnsupported, errors.New("test platform"))}

	exitCode := Run(context.Background(), []string{"diagnose"}, &stdout, &stderr, platform)

	if exitCode != exitUnsupported {
		t.Fatalf("exit code = %d, want %d", exitCode, exitUnsupported)
	}
	if !strings.HasPrefix(stderr.String(), "unsupported:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParseCaptureArgs(t *testing.T) {
	parsed, err := parseCaptureArgs([]string{"--json", "--duration", "250ms"})
	if err != nil {
		t.Fatalf("parseCaptureArgs returned error: %v", err)
	}
	if !parsed.jsonOutput {
		t.Fatal("jsonOutput = false, want true")
	}
	if parsed.duration.Milliseconds() != 250 {
		t.Fatalf("duration = %s, want 250ms", parsed.duration)
	}
}

func TestParseCaptureArgsRejectsNonPositiveDuration(t *testing.T) {
	if _, err := parseCaptureArgs([]string{"--duration=0s"}); err == nil {
		t.Fatal("parseCaptureArgs accepted zero duration")
	}
}

func TestCaptureOutputFailureReturnsRuntimeError(t *testing.T) {
	var stderr bytes.Buffer
	platform := fakePlatform{capture: []lab.CaptureSnapshot{{SchemaVersion: lab.CaptureSchemaVersion}}}

	exitCode := Run(context.Background(), []string{"capture", "--json"}, failingWriter{}, &stderr, platform)

	if exitCode != exitRuntime {
		t.Fatalf("exit code = %d, want %d", exitCode, exitRuntime)
	}
	if !strings.HasPrefix(stderr.String(), "output_error:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestSelfTestInjectJSONSuccess(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	platform := fakePlatform{selfTest: lab.SelfTestResult{
		SchemaVersion:   lab.SelfTestSchemaVersion,
		Passed:          true,
		Sent:            2,
		Observed:        2,
		PointerRestored: true,
	}}

	exitCode := Run(context.Background(), []string{"selftest-inject", "--json"}, &stdout, &stderr, platform)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schemaVersion":"krasis.lab.selftest-inject/v1"`) {
		t.Fatalf("stdout does not contain self-test schema: %q", stdout.String())
	}
}

func TestSelfTestInjectFailureReturnsRuntimeExit(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	platform := fakePlatform{selfTest: lab.SelfTestResult{
		SchemaVersion: lab.SelfTestSchemaVersion,
		Passed:        false,
		Sent:          2,
		Observed:      0,
		FailureCode:   "injected_events_not_observed",
	}}

	exitCode := Run(context.Background(), []string{"selftest-inject", "--json"}, &stdout, &stderr, platform)

	if exitCode != exitRuntime {
		t.Fatalf("exit code = %d, want %d", exitCode, exitRuntime)
	}
	if !strings.Contains(stdout.String(), `"failureCode":"injected_events_not_observed"`) {
		t.Fatalf("stdout does not contain failure code: %q", stdout.String())
	}
}

func TestProbeWritesIndependentAggregateReport(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputPath := filepath.Join(t.TempDir(), "home-a.json")
	platform := fakePlatform{
		diagnostics: lab.Diagnostics{
			SchemaVersion: lab.DiagnoseSchemaVersion,
			OS:            "windows",
			Architecture:  "amd64",
		},
		capture: []lab.CaptureSnapshot{{
			SchemaVersion: lab.CaptureSchemaVersion,
			ElapsedMillis: 25,
			Keyboard:      lab.EventCounts{PhysicalDown: 1, PhysicalUp: 1},
		}},
		selfTest: lab.SelfTestResult{
			SchemaVersion:   lab.SelfTestSchemaVersion,
			Passed:          true,
			Sent:            2,
			Observed:        2,
			PointerRestored: true,
		},
	}

	exitCode := Run(context.Background(), []string{
		"probe",
		"--device", "H-A",
		"--duration", "25ms",
		"--output", outputPath,
	}, &stdout, &stderr, platform)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read probe report: %v", err)
	}
	var report lab.ProbeReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode probe report: %v", err)
	}
	if report.SchemaVersion != lab.ProbeSchemaVersion || report.DeviceAlias != "H-A" || !report.Completed {
		t.Fatalf("unexpected probe report: %+v", report)
	}
	if report.RequestedDurationMs != 25 {
		t.Fatalf("requested duration = %d, want 25", report.RequestedDurationMs)
	}
	for _, forbidden := range []string{`"vkCode"`, `"scanCode"`, `"coordinates"`, `"hostname"`, `"windowTitle"`} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("probe report contains forbidden field %s: %s", forbidden, data)
		}
	}
	if !strings.Contains(stdout.String(), "H-A") || !strings.Contains(stdout.String(), outputPath) {
		t.Fatalf("stdout does not identify probe and report: %q", stdout.String())
	}
}

func TestProbeFailureStillWritesPartialReport(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputPath := filepath.Join(t.TempDir(), "study-b.json")
	platform := fakePlatform{diagnoseErr: errors.New("diagnostics unavailable")}

	exitCode := Run(context.Background(), []string{
		"probe",
		"--device=S-B",
		"--duration=100ms",
		"--output=" + outputPath,
	}, &stdout, &stderr, platform)

	if exitCode != exitRuntime {
		t.Fatalf("exit code = %d, want %d", exitCode, exitRuntime)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read partial report: %v", err)
	}
	var report lab.ProbeReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode partial report: %v", err)
	}
	if report.Completed || report.FailureCode != "diagnose_failed" {
		t.Fatalf("unexpected partial report: %+v", report)
	}
	if !strings.Contains(stderr.String(), "runtime_error:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParseProbeArgsRejectsUnknownAlias(t *testing.T) {
	_, err := parseProbeArgs([]string{
		"--device", "LAPTOP-REAL-NAME",
		"--duration", time.Second.String(),
		"--output", "probe.json",
	})
	if err == nil {
		t.Fatal("parseProbeArgs accepted an unknown device alias")
	}
}
