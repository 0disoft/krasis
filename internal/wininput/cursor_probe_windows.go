//go:build windows

package wininput

import (
	"context"
	"unsafe"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

type cursorGeometry struct{ Left, Top, Width, Height int32 }

var readPlacementGeometry = func() cursorGeometry {
	return cursorGeometry{systemMetric(smXVIRTUALSCREEN), systemMetric(smYVIRTUALSCREEN), systemMetric(smCXVIRTUALSCREEN), systemMetric(smCYVIRTUALSCREEN)}
}

var procGetPhysicalCursorPos = user32.NewProc("GetPhysicalCursorPos")
var readPhysicalCursor = func(p *point) bool {
	ok, _, _ := procGetPhysicalCursorPos.Call(uintptr(unsafe.Pointer(p)))
	return ok != 0
}

// Shared by value copies of a native port. A new lease/process owns a new
// budget; the agent collector independently caps requests across reconnect.
func (port nativeInjectionPort) takePlacementProbe() bool {
	if port.placementProbes == nil {
		return false
	}
	for {
		n := port.placementProbes.Load()
		if n >= handoff.CursorPlacementProbeLimit {
			return false
		}
		if port.placementProbes.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// Sequential snapshots are evidence, not an atomic view or causal attribution.
// Additional queries occur only on mismatch and never set/retry the cursor.
// Failure/cancellation leaves comparisons unknown without changing the receipt.
func observeCursorPlacement(ctx context.Context, requested point, receipt application.CursorPlacement, initialClip cursorBounds, initialGeometry cursorGeometry) *application.CursorPlacementObservation {
	p := &application.CursorPlacementObservation{}
	p.Record(application.CursorTargetAdjusted, requested.X != receipt.TargetX || requested.Y != receipt.TargetY)
	matches := receipt.X == receipt.TargetX && receipt.Y == receipt.TargetY
	p.Record(application.CursorReadbackMatches, matches)
	if matches || ctx.Err() != nil {
		return p
	}
	var clip cursorBounds
	if readCursorBounds(&clip) {
		p.Record(application.CursorClipChanged, clip != initialClip)
	}
	if ctx.Err() != nil {
		return p
	}
	geometry := readPlacementGeometry()
	if geometry.Width > 0 && geometry.Height > 0 {
		p.Record(application.CursorGeometryChanged, geometry != initialGeometry)
	}
	if ctx.Err() != nil {
		return p
	}
	var physical point
	if readPhysicalCursor(&physical) {
		p.Record(application.CursorPhysicalMatchesTarget, physical.X == receipt.TargetX && physical.Y == receipt.TargetY)
		p.Record(application.CursorPhysicalMatchesReadback, physical.X == receipt.X && physical.Y == receipt.Y)
	}
	return p
}
