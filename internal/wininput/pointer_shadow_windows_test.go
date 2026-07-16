//go:build windows

package wininput

import "testing"

func TestPointerShadowDetectsRawOutwardMovementWhileLegacyXIsPinned(t *testing.T) {
	counters, err := newPointerShadowCounters("RIGHT", 0, 1920)
	if err != nil {
		t.Fatal(err)
	}
	counters.observeHook(mouseHookData{Point: point{X: 1918, Y: 540}})
	counters.observeHook(mouseHookData{Point: point{X: 1919, Y: 540}})
	counters.observeHook(mouseHookData{Point: point{X: 1919, Y: 541}})
	counters.observeRaw(rawInputHeader{Device: 1}, rawMouse{LastX: 8})

	observation := counters.snapshot(0)
	if observation.Hook.UnchangedAtEdgeMoves != 1 {
		t.Fatalf("hook counts = %+v", observation.Hook)
	}
	if observation.Raw.OutwardWhileHookAtEdge != 1 || observation.Raw.OutwardPackets != 1 {
		t.Fatalf("raw counts = %+v", observation.Raw)
	}
}

func TestPointerShadowSeparatesAbsoluteAndPrecisionTouchpadPackets(t *testing.T) {
	counters, err := newPointerShadowCounters("LEFT", -1920, 1920)
	if err != nil {
		t.Fatal(err)
	}
	counters.observeRaw(rawInputHeader{}, rawMouse{Flags: mouseMoveAbsolute, LastX: 32767})
	counters.observeRaw(rawInputHeader{}, rawMouse{LastX: -4})

	observation := counters.snapshot(0)
	if observation.Raw.AbsolutePackets != 1 || observation.Raw.RelativePackets != 1 {
		t.Fatalf("raw counts = %+v", observation.Raw)
	}
	if observation.Raw.ZeroDevicePackets != 2 || observation.Raw.OutwardPackets != 1 {
		t.Fatalf("raw counts = %+v", observation.Raw)
	}
}
