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

type touchpadSurfacePlatform struct {
	fakePlatform
	observation lab.TouchpadSurfaceObservation
	err         error
}

func (platform touchpadSurfacePlatform) TouchpadSurface(context.Context, lab.TouchpadSurfaceOptions) (lab.TouchpadSurfaceObservation, error) {
	return platform.observation, platform.err
}

func TestTouchpadSurfaceWritesPointerOnlyAggregateEvidence(t *testing.T) {
	output := filepath.Join(t.TempDir(), "touchpad-surface.json")
	platform := touchpadSurfacePlatform{
		fakePlatform: fakePlatform{diagnostics: lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion, OS: "windows"}},
		observation: lab.TouchpadSurfaceObservation{
			ElapsedMillis: 15_000,
			StopReason:    "duration_elapsed",
			API: lab.TouchpadSurfaceAPIEvidence{
				RegisterAvailable:    true,
				PointerTypeAvailable: true,
				Registered:           true,
			},
			Window: lab.TouchpadSurfaceWindowEvidence{
				Created:            true,
				ForegroundObserved: true,
				Activations:        1,
			},
			Pointer: lab.TouchpadSurfacePointerCounts{
				Down:              2,
				Update:            12,
				Up:                2,
				TypeQueries:       16,
				TouchpadConfirmed: 16,
			},
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"touchpad-surface", "--device", "S-B", "--duration", "15s", "--output", output,
	}, &stdout, &stderr, platform)
	if exitCode != exitSuccess {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	payload, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var report lab.TouchpadSurfaceReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != lab.TouchpadSurfaceSchemaVersion || !report.Completed || !report.Passed {
		t.Fatalf("report = %+v", report)
	}
	if !report.Signature.TouchpadPointerObserved || report.Signature.LegacyWheelObserved || report.Signature.ObservedPath != touchpadSurfacePathPointerOnly {
		t.Fatalf("signature = %+v", report.Signature)
	}
	for _, forbidden := range []string{"pointerId", "pointerID", "pointerX", "pointerY", "deltaX", "deltaY", "devicePath", "windowHandle"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("report contains forbidden field %q: %s", forbidden, payload)
		}
	}
}

func TestTouchpadSurfaceRejectsLegacyOnlyAndForegroundLoss(t *testing.T) {
	observation := lab.TouchpadSurfaceObservation{
		StopReason: "duration_elapsed",
		API: lab.TouchpadSurfaceAPIEvidence{
			RegisterAvailable:    true,
			PointerTypeAvailable: true,
			Registered:           true,
		},
		Window: lab.TouchpadSurfaceWindowEvidence{
			Created:            true,
			ForegroundObserved: true,
			Activations:        1,
			Deactivations:      1,
		},
		Legacy: lab.TouchpadSurfaceLegacyCounts{VerticalWheels: 4},
	}
	signature := touchpadSurfaceSignature(observation)
	if signature.TouchpadPointerObserved || !signature.LegacyWheelObserved || signature.ObservedPath != touchpadSurfacePathLegacyOnly {
		t.Fatalf("signature = %+v", signature)
	}
	failures := touchpadSurfaceEvidenceFailures(observation)
	if !containsString(failures, "touchpad_pointer_not_observed") || !containsString(failures, "touchpad_window_foreground_lost") {
		t.Fatalf("failure codes = %v", failures)
	}
}

func TestTouchpadSurfacePreservesUnsupportedPartialReport(t *testing.T) {
	output := filepath.Join(t.TempDir(), "touchpad-surface.json")
	platform := touchpadSurfacePlatform{
		fakePlatform: fakePlatform{diagnostics: lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion, OS: "windows"}},
		err:          errors.Join(lab.ErrUnsupported, errors.New("RegisterTouchpadCapableWindow unavailable")),
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"touchpad-surface", "--device", "S-B", "--duration", "15s", "--output", output,
	}, &stdout, &stderr, platform)
	if exitCode != exitUnsupported {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var report lab.TouchpadSurfaceReport
	payload, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.Completed || report.Passed || !containsString(report.FailureCodes, "touchpad_api_unsupported") || report.Observation == nil {
		t.Fatalf("report = %+v", report)
	}
}

func TestTouchpadSurfaceRejectsUnsupportedPlatformAndUnsafeOptions(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"touchpad-surface", "--device", "S-B", "--duration", "15s", "--output", filepath.Join(t.TempDir(), "out.json"),
	}, &stdout, &stderr, fakePlatform{})
	if exitCode != exitUnsupported {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}

	invalid := [][]string{
		{"--device", "gram13", "--duration", "15s", "--output", "out.json"},
		{"--device", "S-B", "--duration", "0s", "--output", "out.json"},
		{"--device", "S-B", "--device", "S-A", "--duration", "15s", "--output", "out.json"},
		{"--device", "S-B", "--duration", "15s", "--output", "out.json", "--raw-events"},
	}
	for _, args := range invalid {
		if _, err := parseTouchpadSurfaceArgs(args); err == nil {
			t.Fatalf("parseTouchpadSurfaceArgs(%v) succeeded", args)
		}
	}
}

func TestTouchpadSurfaceDoesNotOverwriteExistingReport(t *testing.T) {
	output := filepath.Join(t.TempDir(), "touchpad-surface.json")
	if err := os.WriteFile(output, []byte("existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"touchpad-surface", "--device", "S-B", "--duration", "15s", "--output", output,
	}, &stdout, &stderr, touchpadSurfacePlatform{})
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
