//go:build windows

package wininput

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

func TestCursorProbeClassifiesChangesAndUnknownsWithoutCursorWrites(t *testing.T) {
	oldClip, oldGeometry, oldPhysical := readCursorBounds, readPlacementGeometry, readPhysicalCursor
	t.Cleanup(func() {
		readCursorBounds, readPlacementGeometry, readPhysicalCursor = oldClip, oldGeometry, oldPhysical
	})
	initialClip := cursorBounds{0, 0, 100, 100}
	initialGeometry := cursorGeometry{0, 0, 100, 100}
	for _, mode := range []string{"matching", "stable", "changed", "physical_target", "physical_readback", "unknown", "cancel_before", "cancel_clip"} {
		t.Run(mode, func(t *testing.T) {
			clipReads, geometryReads, physicalReads := 0, 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			readCursorBounds = func(b *cursorBounds) bool {
				clipReads++
				*b = initialClip
				if mode == "changed" {
					b.Left++
				}
				if mode == "cancel_clip" {
					cancel()
				}
				return mode != "unknown"
			}
			readPlacementGeometry = func() cursorGeometry {
				geometryReads++
				g := initialGeometry
				if mode == "changed" {
					g.Width++
				}
				if mode == "unknown" {
					g.Width = 0
				}
				return g
			}
			readPhysicalCursor = func(p *point) bool {
				physicalReads++
				*p = point{20, 20}
				if mode == "physical_target" {
					*p = point{3, 4}
				}
				if mode == "physical_readback" {
					*p = point{5, 4}
				}
				return mode != "unknown"
			}
			receipt := application.CursorPlacement{TargetX: 3, TargetY: 4, X: 5, Y: 4}
			if mode == "matching" {
				receipt.X = 3
			}
			if mode == "cancel_before" {
				cancel()
			}
			p := observeCursorPlacement(ctx, point{1, 4}, receipt, initialClip, initialGeometry)
			wantKnown, wantFlags := uint8(63), application.CursorTargetAdjusted
			switch mode {
			case "matching":
				wantKnown = 3
				wantFlags |= application.CursorReadbackMatches
			case "changed":
				wantFlags |= application.CursorClipChanged | application.CursorGeometryChanged
			case "physical_target":
				wantFlags |= application.CursorPhysicalMatchesTarget
			case "physical_readback":
				wantFlags |= application.CursorPhysicalMatchesReadback
			case "unknown", "cancel_before":
				wantKnown = 3
			case "cancel_clip":
				wantKnown = 7
			}
			if !p.Valid() || p.Known != wantKnown || p.Flags != wantFlags {
				t.Fatal("comparison classification", p)
			}
			if mode == "matching" || mode == "cancel_before" {
				if clipReads+geometryReads+physicalReads != 0 {
					t.Fatal("unneeded probe queries")
				}
			} else if mode == "cancel_clip" {
				if clipReads != 1 || geometryReads+physicalReads != 0 {
					t.Fatal("queries continued after cancellation")
				}
			} else if clipReads != 1 || geometryReads != 1 || physicalReads != 1 {
				t.Fatal("probe repeated queries")
			}
		})
	}
}

func TestNativeCursorProbeBudgetSharedAcrossCopiesAndNilSafe(t *testing.T) {
	if (nativeInjectionPort{}).takePlacementProbe() {
		t.Fatal("unconfigured probe enabled")
	}
	port := nativeInjectionPort{placementProbes: &atomic.Uint32{}}
	var accepted atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 128; i++ {
		group.Add(1)
		go func(copy nativeInjectionPort) {
			defer group.Done()
			if copy.takePlacementProbe() {
				accepted.Add(1)
			}
		}(port)
	}
	group.Wait()
	if accepted.Load() != handoff.CursorPlacementProbeLimit || port.takePlacementProbe() {
		t.Fatal("worker hard cap escaped")
	}
}

func TestProbedNativePlacementSetsOnlyOnceAndMatchingPathAddsNoQueries(t *testing.T) {
	oldClip, oldGeometry, oldPhysical, oldSet, oldGet := readCursorBounds, readPlacementGeometry, readPhysicalCursor, callSetCursorPos, callGetCursorPos
	t.Cleanup(func() {
		readCursorBounds, readPlacementGeometry, readPhysicalCursor, callSetCursorPos, callGetCursorPos = oldClip, oldGeometry, oldPhysical, oldSet, oldGet
	})
	left, top := systemMetric(smXVIRTUALSCREEN), systemMetric(smYVIRTUALSCREEN)
	if systemMetric(smCXVIRTUALSCREEN) < 10 || systemMetric(smCYVIRTUALSCREEN) < 10 {
		t.Skip("no usable test screen geometry")
	}
	for _, mode := range []string{"disabled", "matching", "mismatch", "exhausted"} {
		t.Run(mode, func(t *testing.T) {
			sets, reads, clips, geometries, physicals := 0, 0, 0, 0, 0
			readCursorBounds = func(b *cursorBounds) bool { clips++; *b = cursorBounds{left, top, left + 10, top + 10}; return true }
			readPlacementGeometry = func() cursorGeometry { geometries++; return cursorGeometry{left, top, 10, 10} }
			readPhysicalCursor = func(p *point) bool { physicals++; *p = point{left + 2, top + 1}; return true }
			callSetCursorPos = func(...uintptr) (uintptr, uintptr, error) { sets++; return 1, 0, nil }
			callGetCursorPos = func(p *point) (uintptr, uintptr, error) {
				reads++
				*p = point{left + 1, top + 1}
				if mode == "mismatch" {
					p.X++
				}
				return 1, 0, nil
			}
			port := NewInjectionPort().(nativeInjectionPort)
			place := port.ProbeCursorPlacement
			if mode == "disabled" {
				place = port.PlaceCursor
			}
			if mode == "exhausted" {
				port.placementProbes.Store(handoff.CursorPlacementProbeLimit)
			}
			p, err := place(context.Background(), left+1, top+1)
			if err != nil || sets != 1 || reads != 1 {
				t.Fatal("placement repeated or changed", err)
			}
			wantQueries := 1
			if mode == "mismatch" {
				wantQueries = 2
			}
			if clips != wantQueries || geometries != wantQueries || physicals != wantQueries-1 {
				t.Fatal("extra native queries")
			}
			if (p.Observation == nil) != (mode == "disabled" || mode == "exhausted") {
				t.Fatal("observation budget lost")
			}
		})
	}
}
