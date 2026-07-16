package labcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/lab"
)

type pointerShadowPlatform struct {
	fakePlatform
	observation lab.PointerShadowObservation
	err         error
}

func (platform pointerShadowPlatform) PointerShadow(context.Context, lab.PointerShadowOptions) (lab.PointerShadowObservation, error) {
	return platform.observation, platform.err
}

func TestPointerShadowWritesAggregateEdgeSignature(t *testing.T) {
	output := filepath.Join(t.TempDir(), "shadow.json")
	platform := pointerShadowPlatform{
		fakePlatform: fakePlatform{diagnostics: lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion, OS: "windows"}},
		observation: lab.PointerShadowObservation{
			ElapsedMillis: 10_000,
			Raw: lab.PointerShadowRawCounts{
				RelativePackets:        50,
				OutwardPackets:         40,
				OutwardWhileHookAtEdge: 35,
			},
			Hook: lab.PointerShadowHookCounts{
				PhysicalMoves:        45,
				EdgeMoves:            30,
				UnchangedAtEdgeMoves: 25,
			},
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"pointer-shadow", "--device", "S-A", "--edge", "RIGHT", "--duration", "10s", "--output", output,
	}, &stdout, &stderr, platform)
	if exitCode != exitSuccess {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var report lab.PointerShadowReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Completed || !report.Passed || !report.Signature.Observed || report.SchemaVersion != lab.PointerShadowSchemaVersion {
		t.Fatalf("report = %+v", report)
	}
	for _, forbidden := range []string{"pointerX", "pointerY", "scanCode", "trajectory", "windowTitle"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("report contains forbidden field %q: %s", forbidden, data)
		}
	}
}

func TestPointerShadowPreservesIncompleteEvidence(t *testing.T) {
	output := filepath.Join(t.TempDir(), "shadow.json")
	platform := pointerShadowPlatform{
		fakePlatform: fakePlatform{diagnostics: lab.Diagnostics{SchemaVersion: lab.DiagnoseSchemaVersion, OS: "windows"}},
		observation: lab.PointerShadowObservation{
			ElapsedMillis: 10_000,
			Raw:           lab.PointerShadowRawCounts{RelativePackets: 5},
			Hook:          lab.PointerShadowHookCounts{PhysicalMoves: 5},
		},
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run(t.Context(), []string{
		"pointer-shadow", "--device", "S-B", "--edge", "LEFT", "--duration", "10s", "--output", output,
	}, &stdout, &stderr, platform)
	if exitCode != exitRuntime {
		t.Fatalf("Run() code=%d stdout=%q stderr=%q", exitCode, stdout.String(), stderr.String())
	}
	var report lab.PointerShadowReport
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if !report.Completed || report.Passed || len(report.FailureCodes) == 0 {
		t.Fatalf("report = %+v", report)
	}
}

func TestParsePointerShadowArgsRejectsUnsafeShape(t *testing.T) {
	if _, err := parsePointerShadowArgs([]string{"--device", "S-A", "--edge", "TOP", "--duration", "10s", "--output", "x.json"}); err == nil {
		t.Fatal("parsePointerShadowArgs accepted unsupported edge")
	}
	if _, err := parsePointerShadowArgs([]string{"--device", "host-name", "--edge", "RIGHT", "--duration", "10s", "--output", "x.json"}); err == nil {
		t.Fatal("parsePointerShadowArgs accepted arbitrary device name")
	}
}
