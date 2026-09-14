//go:build windows

package wininput

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestPhysicalCoordinatesInitialization(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		initial, afterV2, afterV1 int32
		calls                     int
		fail                      bool
	}{
		{"already configured", 2, 2, 2, 0, false},
		{"v2", 0, 2, 2, 1, false},
		{"v1 fallback", 0, 0, 2, 2, false},
		{"incompatible existing mode", 1, 1, 1, 2, true},
		{"unavailable", -1, -1, -1, 2, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state, calls := tt.initial, 0
			err := initializePhysicalCoordinates(func() int32 { return state }, func() { calls++; state = tt.afterV2 }, func() { calls++; state = tt.afterV1 })
			if (err != nil) != tt.fail || calls != tt.calls {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

// Isolate process-global DPI changes from every other test. The child only
// queries geometry; it never creates a product agent, hooks or injected input.
func TestPhysicalCoordinatesNativeProcess(t *testing.T) {
	if os.Getenv("KRASIS_DPI_TEST_CHILD") == "1" {
		if err := InitializePhysicalCoordinates(); err != nil {
			t.Fatal(err)
		}
		if err := InitializePhysicalCoordinates(); err != nil {
			t.Fatal("not idempotent:", err)
		}
		width, height, awareness := DisplayCoordinates()
		if awareness != 2 {
			t.Fatalf("awareness=%d", awareness)
		}
		if systemMetric(smCMONITORS) == 1 {
			if width == 0 || height == 0 || int32(width) != systemMetric(smCXVIRTUALSCREEN) || int32(height) != systemMetric(smCYVIRTUALSCREEN) {
				t.Fatal("screen metrics and physical dimensions disagree")
			}
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPhysicalCoordinatesNativeProcess$")
	cmd.Env = append(os.Environ(), "KRASIS_DPI_TEST_CHILD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated DPI probe: %v\n%s", err, output)
	}
}

func TestPhysicalBoundaryDoesNotUse125PercentLogicalWidth(t *testing.T) {
	d, err := control.NewBoundaryDetector(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 1920, RearmDistance: 32})
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []int32{1535, 1581, 1918, 1919} {
		_, fired, err := d.Observe(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, x)
		if err != nil || fired != (x == 1919) {
			t.Fatalf("x=%d fired=%t err=%v", x, fired, err)
		}
	}
}
