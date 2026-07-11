package labcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/lab"
)

type guidedPlatform struct {
	captures    []lab.CaptureSnapshot
	next        int
	selfTest    lab.SelfTestResult
	selfTestErr error
}

func (p *guidedPlatform) Diagnose(context.Context) (lab.Diagnostics, error) {
	return lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion}, nil
}

func (p *guidedPlatform) Capture(_ context.Context, _ lab.CaptureOptions, report func(lab.CaptureSnapshot)) error {
	snapshot := p.captures[p.next]
	p.next++
	report(snapshot)
	return nil
}

func (p *guidedPlatform) SelfTestInject(context.Context) (lab.SelfTestResult, error) {
	if p.selfTestErr != nil {
		return lab.SelfTestResult{}, p.selfTestErr
	}
	if p.selfTest.SchemaVersion != "" {
		return p.selfTest, nil
	}
	return lab.SelfTestResult{
		SchemaVersion:   lab.SelfTestSchemaVersion,
		Passed:          true,
		Sent:            2,
		Observed:        2,
		PointerRestored: true,
	}, nil
}

func TestGuidedProbeContinuesPhysicalStagesAfterSelfTestFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputPath := filepath.Join(t.TempDir(), "guided-selftest-failed.json")
	platform := &guidedPlatform{
		captures: guidedPassingCaptures(),
		selfTest: lab.SelfTestResult{
			SchemaVersion: lab.SelfTestSchemaVersion,
			Sent:          2,
			FailureCode:   "injected_events_not_observed",
		},
	}

	exitCode := Run(context.Background(), []string{
		"guided-probe",
		"--device", "S-B",
		"--stage-duration", "1ms",
		"--output", outputPath,
	}, &stdout, &stderr, platform)

	if exitCode != exitRuntime {
		t.Fatalf("exit code = %d, want %d", exitCode, exitRuntime)
	}
	report := readGuidedReport(t, outputPath)
	if report.Completed || report.FailureCode != "selftest_failed" || len(report.Stages) != 3 {
		t.Fatalf("unexpected guided report: %+v", report)
	}
	for _, stage := range report.Stages {
		if !stage.Passed {
			t.Fatalf("physical stage did not complete after self-test failure: %+v", stage)
		}
	}
}

func TestGuidedProbeWritesPassedStageEvidence(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputPath := filepath.Join(t.TempDir(), "guided.json")
	platform := &guidedPlatform{captures: guidedPassingCaptures()}

	exitCode := Run(context.Background(), []string{
		"guided-probe",
		"--device", "H-B",
		"--stage-duration", "1ms",
		"--output", outputPath,
	}, &stdout, &stderr, platform)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	report := readGuidedReport(t, outputPath)
	if !report.Completed || len(report.Stages) != 3 {
		t.Fatalf("unexpected guided report: %+v", report)
	}
	for _, stage := range report.Stages {
		if !stage.Passed || len(stage.FailureCodes) != 0 {
			t.Fatalf("unexpected guided stage: %+v", stage)
		}
	}
}

func TestGuidedProbeRecordsIncompletePhysicalInput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputPath := filepath.Join(t.TempDir(), "guided-incomplete.json")
	captures := guidedPassingCaptures()
	captures[2].Mouse.PhysicalWheel = 0
	platform := &guidedPlatform{captures: captures}

	exitCode := Run(context.Background(), []string{
		"guided-probe",
		"--device=S-A",
		"--stage-duration=1ms",
		"--output=" + outputPath,
	}, &stdout, &stderr, platform)

	if exitCode != exitRuntime {
		t.Fatalf("exit code = %d, want %d", exitCode, exitRuntime)
	}
	report := readGuidedReport(t, outputPath)
	if report.Completed || report.FailureCode != "guided_input_incomplete" {
		t.Fatalf("unexpected guided report: %+v", report)
	}
	if report.Stages[2].Passed || len(report.Stages[2].FailureCodes) != 1 || report.Stages[2].FailureCodes[0] != "wheel_not_observed" {
		t.Fatalf("unexpected wheel stage: %+v", report.Stages[2])
	}
}

func TestGuidedProbeRejectsUnknownDeviceAlias(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{
		"guided-probe",
		"--device", "HOME-LAPTOP",
		"--stage-duration", "1s",
		"--output", filepath.Join(t.TempDir(), "guided.json"),
	}, &stdout, &stderr, &guidedPlatform{})

	if exitCode != exitUsage {
		t.Fatalf("exit code = %d, want %d", exitCode, exitUsage)
	}
}

func guidedPassingCaptures() []lab.CaptureSnapshot {
	return []lab.CaptureSnapshot{
		{
			SchemaVersion: lab.CaptureSchemaVersion,
			Keyboard:      lab.EventCounts{PhysicalDown: 1, PhysicalUp: 1},
		},
		{
			SchemaVersion: lab.CaptureSchemaVersion,
			Mouse:         lab.MouseCounts{PhysicalMove: 1},
		},
		{
			SchemaVersion: lab.CaptureSchemaVersion,
			Mouse: lab.MouseCounts{
				PhysicalWheel:      1,
				PhysicalButtonDown: 1,
				PhysicalButtonUp:   1,
			},
		},
	}
}

func readGuidedReport(t *testing.T, path string) lab.GuidedProbeReport {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read guided report: %v", err)
	}
	var report lab.GuidedProbeReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode guided report: %v", err)
	}
	return report
}
