//go:build windows

package wininput

import (
	"context"
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

func TestWindowsEntryPreservesNormalizedHeightThroughInjectionPort(t *testing.T) {
	rejectOrdinaryCursorIO(t)
	for _, h := range []control.EdgeHeight{1, 32769, 65536} {
		p := &delegatedInput{position: point{X: 100, Y: 200}}
		a, err := NewInputAdapterWithInjection(10, 1920, p)
		if err != nil {
			t.Fatal(err)
		}
		ctx := control.WithEdgeHeight(context.Background(), h)
		if err := a.PrepareEntry(ctx, control.ScreenEdgeRight); err != nil {
			t.Fatal(err)
		}
		want, err := h.Coordinate(systemMetric(smYVIRTUALSCREEN), systemMetric(smCYVIRTUALSCREEN))
		if err != nil || p.position.Y != want {
			t.Fatal("Windows entry still centered", p.position.Y, want, err)
		}
		a.CommitEntry()
	}
}

func TestWindowsBoundaryCapturesHeightFromCoherentHookPosition(t *testing.T) {
	d, err := control.NewBoundaryDetector(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 1920, RearmDistance: 10})
	if err != nil {
		t.Fatal(err)
	}
	e, err := control.NewBoundaryIntentEmitter(d)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSourceSession(8, &recordingSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	emergency, err := NewEmergencyBoundaryObserver(s, e)
	if err != nil {
		t.Fatal(err)
	}
	state := &sourcePointerState{top: -100, height: 1080}
	state.observe(point{X: 1919, Y: -100})
	if err := routeRawMouse(s, emergency, state, rawInputHeader{}, rawMouse{LastX: 1}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-e.Intents():
		if got.Height != 1 {
			t.Fatal("hook height lost before intent", got)
		}
	default:
		t.Fatal("no boundary intent")
	}
}
