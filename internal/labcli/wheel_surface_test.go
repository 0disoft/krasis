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

	"github.com/0disoft/krasis/internal/lab"
)

type wheelSurfacePlatform struct {
	fakePlatform
	observation lab.WheelSurfaceObservation
	err         error
}

func (platform wheelSurfacePlatform) WheelSurface(context.Context, lab.WheelSurfaceOptions) (lab.WheelSurfaceObservation, error) {
	return platform.observation, platform.err
}

func TestWheelSurfaceWritesHealthyAggregateWithoutRequiringPhysicalWheel(t *testing.T) {
	output := filepath.Join(t.TempDir(), "wheel-surface.json")
	platform := wheelSurfacePlatform{
		fakePlatform: fakePlatform{diagnostics: lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion, OS: "windows"}},
		observation: lab.WheelSurfaceObservation{
			ElapsedMillis: 10_000,
			Hook: lab.WheelSurfaceHookCounts{
				PhysicalMoves:       20,
				PhysicalButtonDowns: 2,
				PhysicalButtonUps:   2,
			},
			Raw:      lab.WheelSurfaceRawCounts{Packets: 24, ZeroDevicePackets: 24},
			Sentinel: lab.WheelSurfaceSentinelCounts{Requested: 4, Inserted: 4, Observed: 4, Swallowed: 4},
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"wheel-surface", "--device", "S-B", "--duration", "10s", "--output", output,
	}, &stdout, &stderr, platform)
	if exitCode != exitSuccess {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var report lab.WheelSurfaceReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != lab.WheelSurfaceSchemaVersion || !report.Completed || !report.Passed {
		t.Fatalf("report = %+v", report)
	}
	if !report.Signature.HookHealthConfirmed || !report.Signature.PhysicalInteractionObserved || report.Signature.ObservedPath != wheelSurfacePathNeither {
		t.Fatalf("signature = %+v", report.Signature)
	}
	for _, forbidden := range []string{"pointerX", "pointerY", "deltaX", "deltaY", "scanCode", "trajectory", "windowTitle", "devicePath"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("report contains forbidden field %q: %s", forbidden, payload)
		}
	}
}

func TestWheelSurfaceClassifiesRawOnlyWheelEvidence(t *testing.T) {
	signature := wheelSurfaceSignature(lab.WheelSurfaceObservation{
		Raw:      lab.WheelSurfaceRawCounts{VerticalWheelPackets: 3},
		Sentinel: lab.WheelSurfaceSentinelCounts{Requested: 4, Inserted: 4, Observed: 4, Swallowed: 4},
	})
	if !signature.HookHealthConfirmed || signature.HookWheelObserved || !signature.RawWheelObserved || signature.ObservedPath != wheelSurfacePathRawOnly {
		t.Fatalf("signature = %+v", signature)
	}
}

func TestWheelSurfaceClassifiesSynthesizedHookWheelEvidence(t *testing.T) {
	signature := wheelSurfaceSignature(lab.WheelSurfaceObservation{
		Hook: lab.WheelSurfaceHookCounts{
			PhysicalMoves:             1,
			SynthesizedVerticalWheels: 2,
		},
		Sentinel: lab.WheelSurfaceSentinelCounts{Requested: 4, Inserted: 4, Observed: 4, Swallowed: 4},
	})
	if !signature.HookWheelObserved || signature.HookPhysicalWheelObserved || !signature.HookSynthesizedWheelObserved || signature.RawWheelObserved || signature.ObservedPath != wheelSurfacePathHookOnly {
		t.Fatalf("signature = %+v", signature)
	}
}

func TestWheelSurfaceRejectsMissingPhysicalExercise(t *testing.T) {
	failures := wheelSurfaceEvidenceFailures(lab.WheelSurfaceObservation{
		Sentinel: lab.WheelSurfaceSentinelCounts{Requested: 4, Inserted: 4, Observed: 4, Swallowed: 4},
	})
	if !containsString(failures, "physical_interaction_not_observed") {
		t.Fatalf("failure codes = %v", failures)
	}
}

func TestWheelSurfacePreservesUnhealthySentinelEvidence(t *testing.T) {
	output := filepath.Join(t.TempDir(), "wheel-surface.json")
	platform := wheelSurfacePlatform{
		fakePlatform: fakePlatform{diagnostics: lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion, OS: "windows"}},
		observation: lab.WheelSurfaceObservation{
			ElapsedMillis: 10_000,
			Sentinel:      lab.WheelSurfaceSentinelCounts{Requested: 4, Inserted: 4, Observed: 2, Swallowed: 2},
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"wheel-surface", "--device", "S-B", "--duration", "10s", "--output", output,
	}, &stdout, &stderr, platform)
	if exitCode != exitRuntime {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var report lab.WheelSurfaceReport
	payload, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Completed || report.Passed || report.Signature.HookHealthConfirmed {
		t.Fatalf("report = %+v", report)
	}
	if !containsString(report.FailureCodes, "sentinel_not_observed") {
		t.Fatalf("failure codes = %v", report.FailureCodes)
	}
}

func TestWheelSurfacePreservesPartialReportOnRuntimeFailure(t *testing.T) {
	output := filepath.Join(t.TempDir(), "wheel-surface.json")
	platform := wheelSurfacePlatform{
		fakePlatform: fakePlatform{diagnostics: lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion, OS: "windows"}},
		observation: lab.WheelSurfaceObservation{
			ElapsedMillis: 500,
			Sentinel:      lab.WheelSurfaceSentinelCounts{Requested: 2, Inserted: 1, Observed: 1, Swallowed: 1},
		},
		err: errors.New("raw input reader stopped"),
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"wheel-surface", "--device", "S-B", "--duration", "10s", "--output", output,
	}, &stdout, &stderr, platform)
	if exitCode != exitRuntime || !strings.Contains(stderr.String(), "raw input reader stopped") {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var report lab.WheelSurfaceReport
	payload, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.Completed || len(report.FailureCodes) != 1 || report.FailureCodes[0] != "wheel_surface_capture_failed" || report.Observation == nil {
		t.Fatalf("report = %+v", report)
	}
}

func TestWheelSurfaceRejectsUnsupportedPlatformAndUnsafeOptions(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"wheel-surface", "--device", "S-B", "--duration", "10s", "--output", filepath.Join(t.TempDir(), "out.json"),
	}, &stdout, &stderr, fakePlatform{})
	if exitCode != exitUnsupported {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	invalid := [][]string{
		{"--device", "gram13", "--duration", "10s", "--output", "out.json"},
		{"--device", "S-B", "--duration", "0s", "--output", "out.json"},
		{"--device", "S-B", "--device", "S-A", "--duration", "10s", "--output", "out.json"},
		{"--device", "S-B", "--duration", "10s", "--output", "out.json", "--raw-events"},
	}
	for _, args := range invalid {
		if _, err := parseWheelSurfaceArgs(args); err == nil {
			t.Fatalf("parseWheelSurfaceArgs(%v) succeeded", args)
		}
	}
}

func TestWheelSurfaceDoesNotOverwriteExistingReport(t *testing.T) {
	output := filepath.Join(t.TempDir(), "wheel-surface.json")
	if err := os.WriteFile(output, []byte("existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	platform := wheelSurfacePlatform{
		fakePlatform: fakePlatform{diagnostics: lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion, OS: "windows"}},
		observation: lab.WheelSurfaceObservation{
			Sentinel: lab.WheelSurfaceSentinelCounts{Requested: 4, Inserted: 4, Observed: 4, Swallowed: 4},
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"wheel-surface", "--device", "S-B", "--duration", "10s", "--output", output,
	}, &stdout, &stderr, platform)
	if exitCode != exitUsage || !strings.Contains(stderr.String(), "already exists") {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "existing\n" {
		t.Fatalf("existing report was overwritten: %q", payload)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
