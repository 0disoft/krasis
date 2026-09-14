//go:build windows

package wininput

import (
	"errors"
	"testing"
	"unsafe"
)

func TestWheelSurfaceHookSeparatesPhysicalSynthesizedAndSentinelEvents(t *testing.T) {
	counters := &wheelSurfaceCounters{}
	physicalMessages := []uint32{
		wmMouseMove,
		wmLButtonDown,
		wmLButtonUp,
		wmMouseWheel,
		wmMouseHWheel,
	}
	for _, message := range physicalMessages {
		if counters.observeHook(message, mouseHookData{}) {
			t.Fatalf("physical message %#x was swallowed", message)
		}
	}
	if counters.observeHook(wmMouseWheel, mouseHookData{Flags: llmhfInjected}) {
		t.Fatal("untagged synthesized wheel was swallowed")
	}
	if counters.observeHook(wmMouseHWheel, mouseHookData{ExtraInfo: krasisInjectedExtraInfo}) {
		t.Fatal("Krasis-tagged synthesized wheel was swallowed")
	}
	if !counters.observeHook(wmMouseMove, mouseHookData{ExtraInfo: wheelSurfaceSentinelExtraInfo}) {
		t.Fatal("wheel sentinel was not swallowed")
	}
	if counters.observeHook(wmMouseWheel, mouseHookData{ExtraInfo: wheelSurfaceSentinelExtraInfo}) {
		t.Fatal("non-move sentinel-tagged event was swallowed")
	}

	snapshot := counters.snapshot(0)
	if snapshot.Hook.PhysicalMoves != 1 || snapshot.Hook.PhysicalButtonDowns != 1 || snapshot.Hook.PhysicalButtonUps != 1 {
		t.Fatalf("physical hook counts = %+v", snapshot.Hook)
	}
	if snapshot.Hook.PhysicalVerticalWheels != 1 || snapshot.Hook.PhysicalHorizontalWheels != 1 {
		t.Fatalf("physical wheel counts = %+v", snapshot.Hook)
	}
	if snapshot.Hook.SynthesizedVerticalWheels != 1 || snapshot.Hook.SynthesizedHorizontalWheels != 1 {
		t.Fatalf("synthesized wheel counts = %+v", snapshot.Hook)
	}
	if snapshot.Sentinel.Observed != 1 || snapshot.Sentinel.Swallowed != 1 {
		t.Fatalf("sentinel counts = %+v", snapshot.Sentinel)
	}
}

func TestWheelSurfaceRawWheelParsingPreservesSignedTotalsAndZeroDevicePackets(t *testing.T) {
	counters := &wheelSurfaceCounters{}
	counters.observeRaw(rawInputHeader{Device: 0}, rawMouse{Buttons: packRawWheel(rawMouseWheel, 120)})
	counters.observeRaw(rawInputHeader{Device: 9}, rawMouse{Buttons: packRawWheel(rawMouseWheel, -120)})
	counters.observeRaw(rawInputHeader{Device: 9}, rawMouse{Buttons: packRawWheel(rawMouseHWheel, 240)})
	counters.observeRaw(rawInputHeader{Device: 9}, rawMouse{Buttons: packRawWheel(rawMouseWheel|rawMouseHWheel, -32768)})
	counters.observeRaw(rawInputHeader{Device: 0}, rawMouse{
		Buttons:          packRawWheel(rawMouseWheel, 1),
		ExtraInformation: uint32(wheelSurfaceSentinelExtraInfo),
	})

	raw := counters.snapshot(0).Raw
	if raw.Packets != 5 || raw.ZeroDevicePackets != 2 || raw.TaggedInjectedPackets != 1 {
		t.Fatalf("raw packet counts = %+v", raw)
	}
	if raw.VerticalWheelPackets != 3 || raw.HorizontalWheelPackets != 2 {
		t.Fatalf("raw wheel packet counts = %+v", raw)
	}
	if raw.VerticalDeltaTotal != -32768 || raw.HorizontalDeltaTotal != -32528 {
		t.Fatalf("raw signed totals = %+v", raw)
	}
	if raw.VerticalDeltaMagnitude != 33008 || raw.HorizontalDeltaMagnitude != 33008 {
		t.Fatalf("raw magnitudes = %+v", raw)
	}
}

func TestSendWheelSurfaceSentinelUsesTinyBalancedTaggedMoveEvents(t *testing.T) {
	previous := callSendInput
	t.Cleanup(func() { callSendInput = previous })
	callSendInput = func(count uintptr, inputs *input, size uintptr) (uintptr, uintptr, error) {
		if count != wheelSurfaceSentinelEventsPerPulse {
			t.Fatalf("SendInput count=%d", count)
		}
		if size != unsafe.Sizeof(input{}) {
			t.Fatalf("SendInput size=%d want=%d", size, unsafe.Sizeof(input{}))
		}
		events := unsafe.Slice(inputs, int(count))
		if events[0].Type != inputMouse || events[1].Type != inputMouse {
			t.Fatalf("event types = %d,%d", events[0].Type, events[1].Type)
		}
		wantFlags := uint32(mouseEventMove | mouseEventMoveNoCoalesce)
		if events[0].Mouse.Flags != wantFlags || events[1].Mouse.Flags != wantFlags {
			t.Fatalf("event flags = %#x,%#x", events[0].Mouse.Flags, events[1].Mouse.Flags)
		}
		if events[0].Mouse.DX != 1 || events[1].Mouse.DX != -1 || events[0].Mouse.DY != 0 || events[1].Mouse.DY != 0 {
			t.Fatalf("event deltas = (%d,%d),(%d,%d)", events[0].Mouse.DX, events[0].Mouse.DY, events[1].Mouse.DX, events[1].Mouse.DY)
		}
		if events[0].Mouse.ExtraInfo != wheelSurfaceSentinelExtraInfo || events[1].Mouse.ExtraInfo != wheelSurfaceSentinelExtraInfo {
			t.Fatalf("event tags = %#x,%#x", events[0].Mouse.ExtraInfo, events[1].Mouse.ExtraInfo)
		}
		return count, 0, nil
	}
	requested, inserted, err := sendWheelSurfaceSentinelPulse()
	if err != nil || requested != 2 || inserted != 2 {
		t.Fatalf("sendWheelSurfaceSentinelPulse() requested=%d inserted=%d err=%v", requested, inserted, err)
	}
}

func TestSendWheelSurfaceSentinelRejectsPartialAndImpossibleInsertion(t *testing.T) {
	previous := callSendInput
	t.Cleanup(func() { callSendInput = previous })

	for _, inserted := range []uintptr{0, 1, 3} {
		callSendInput = func(uintptr, *input, uintptr) (uintptr, uintptr, error) {
			return inserted, 0, errors.New("sentinel insertion failed")
		}
		requested, got, err := sendWheelSurfaceSentinelPulse()
		if requested != 2 || got != uint64(inserted) || err == nil {
			t.Fatalf("inserted=%d requested=%d got=%d err=%v", inserted, requested, got, err)
		}
	}
}

func packRawWheel(flags uint16, delta int16) uint32 {
	return uint32(flags) | uint32(uint16(delta))<<16
}
