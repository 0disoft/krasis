//go:build windows

package wininput

import "testing"

func TestTouchpadSurfaceCountersClassifyPointerTypesAndLegacyWheel(t *testing.T) {
	counters := &touchpadSurfaceCounters{}
	queries := map[uint32]struct {
		pointerType uint32
		ok          bool
	}{
		1: {pointerType: ptTouchpad, ok: true},
		2: {pointerType: 4, ok: true},
		3: {ok: false},
	}
	query := func(pointerID uint32) (uint32, bool) {
		result := queries[pointerID]
		return result.pointerType, result.ok
	}
	counters.observePointer(wmPointerDown, 1, query)
	counters.observePointer(wmPointerUpdate, 1, query)
	counters.observePointer(wmPointerUp, 2, query)
	counters.observePointer(wmPointerHorizontalWheel, 3, query)
	counters.legacyVertical++
	counters.legacyHorizontal++

	snapshot := counters.pointerSnapshot()
	if snapshot.Down != 1 || snapshot.Update != 1 || snapshot.Up != 1 || snapshot.HorizontalWheel != 1 {
		t.Fatalf("message counts = %+v", snapshot)
	}
	if snapshot.TypeQueries != 4 || snapshot.TypeQueryFailures != 1 || snapshot.TouchpadConfirmed != 2 || snapshot.OtherConfirmed != 1 {
		t.Fatalf("type counts = %+v", snapshot)
	}
	if counters.legacyVertical != 1 || counters.legacyHorizontal != 1 {
		t.Fatalf("legacy counts = %d/%d", counters.legacyVertical, counters.legacyHorizontal)
	}
}

func TestTouchpadSurfaceStopReasonIsStable(t *testing.T) {
	tests := map[int32]string{
		touchpadStopDuration:     "duration_elapsed",
		touchpadStopCanceled:     "canceled",
		touchpadStopWindowClosed: "window_closed",
		touchpadStopEscape:       "escape",
		0:                        "message_loop_stopped",
	}
	for code, want := range tests {
		if got := touchpadSurfaceStopReason(code); got != want {
			t.Fatalf("touchpadSurfaceStopReason(%d)=%q want=%q", code, got, want)
		}
	}
}
