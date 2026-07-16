//go:build windows

package wininput

import (
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestMapRawMouseUsesRelativeMovementAndAcceptsZeroDeviceInput(t *testing.T) {
	actions, err := mapRawMouse(rawMouse{LastX: -7, LastY: 3})
	if err != nil || actions.count != 1 {
		t.Fatalf("mapRawMouse() count=%d error=%v", actions.count, err)
	}
	want := control.InputAction{Kind: control.InputPointerMove, DeltaX: -7, DeltaY: 3}
	if actions.values[0] != want {
		t.Fatalf("action = %+v, want %+v", actions.values[0], want)
	}

	session := newInactiveSourceSession(t)
	observer := &recordingBoundaryObserver{}
	state := newSourcePointerState(point{X: 0, Y: 400})
	if err := routeRawMouse(session, observer, state, rawInputHeader{Device: 0}, rawMouse{LastX: -7}); err != nil {
		t.Fatal(err)
	}
	if observer.calls != 1 || observer.pointerX != 0 || observer.action.DeltaX != -7 {
		t.Fatalf("observer = %+v", observer)
	}
}

func TestMapRawMouseRejectsDuplicateAndUnsupportedMovementSources(t *testing.T) {
	tests := []rawMouse{
		{},
		{Flags: mouseMoveAbsolute, LastX: 32767},
		{LastX: 4, ExtraInformation: uint32(krasisInjectedExtraInfo)},
		{LastX: 4, ExtraInformation: uint32(selfTestExtraInfo)},
		{Buttons: uint32(0x0400) | uint32(uint16(120))<<16},
		{Buttons: uint32(0x0800) | uint32(uint16(120))<<16},
	}
	for _, mouse := range tests {
		actions, err := mapRawMouse(mouse)
		if err != nil || actions.count != 0 {
			t.Fatalf("mouse=%+v actions=%+v error=%v", mouse, actions, err)
		}
	}
}

func TestMapRawMousePreservesMoveAndButtonOrderWhileIgnoringWheel(t *testing.T) {
	const rawWheelFlags = uint32(0x0c00)
	negativeWheel := uint32(uint16(0xff88)) << 16
	mouse := rawMouse{
		LastX:   4,
		LastY:   -2,
		Buttons: uint32(rawMouseLeftButtonDown|rawMouseButton5Up) | rawWheelFlags | negativeWheel,
	}
	actions, err := mapRawMouse(mouse)
	if err != nil {
		t.Fatal(err)
	}
	want := []control.InputAction{
		{Kind: control.InputPointerMove, DeltaX: 4, DeltaY: -2},
		{Kind: control.InputButtonDown, Code: MouseButtonLeft},
		{Kind: control.InputButtonUp, Code: MouseButtonX2},
	}
	if actions.count != len(want) {
		t.Fatalf("action count = %d, want %d", actions.count, len(want))
	}
	for index, expected := range want {
		if actions.values[index] != expected {
			t.Fatalf("action[%d] = %+v, want %+v", index, actions.values[index], expected)
		}
	}
}

func TestMapHookMouseWheelOwnsVerticalAndHorizontalWheel(t *testing.T) {
	vertical, emitted, err := mapHookMouseWheel(wmMouseWheel, mouseHookData{MouseData: uint32(uint16(0xff88)) << 16})
	if err != nil || !emitted || vertical != (control.InputAction{Kind: control.InputWheel, DeltaY: -120}) {
		t.Fatalf("vertical wheel = %+v emitted=%t error=%v", vertical, emitted, err)
	}
	horizontal, emitted, err := mapHookMouseWheel(wmMouseHWheel, mouseHookData{MouseData: uint32(uint16(120)) << 16})
	if err != nil || !emitted || horizontal != (control.InputAction{Kind: control.InputWheel, DeltaX: 120}) {
		t.Fatalf("horizontal wheel = %+v emitted=%t error=%v", horizontal, emitted, err)
	}
}

func TestMapHookMouseWheelAcceptsUntaggedSynthesizedWheelAndKeepsOwnedUnknownAndZeroEventsLocal(t *testing.T) {
	synthesized := mouseHookData{MouseData: uint32(uint16(120)) << 16, Flags: llmhfInjected}
	action, emitted, err := mapHookMouseWheel(wmMouseWheel, synthesized)
	if err != nil || !emitted || action != (control.InputAction{Kind: control.InputWheel, DeltaY: 120}) {
		t.Fatalf("synthesized wheel = %+v emitted=%t error=%v", action, emitted, err)
	}
	tests := []struct {
		message uint32
		data    mouseHookData
	}{
		{message: wmMouseWheel, data: mouseHookData{MouseData: uint32(uint16(120)) << 16, ExtraInfo: krasisInjectedExtraInfo}},
		{message: wmMouseWheel, data: mouseHookData{MouseData: uint32(uint16(120)) << 16, ExtraInfo: selfTestExtraInfo}},
		{message: wmMouseWheel, data: mouseHookData{}},
		{message: wmMouseMove, data: mouseHookData{MouseData: uint32(uint16(120)) << 16}},
	}
	for _, test := range tests {
		action, emitted, err := mapHookMouseWheel(test.message, test.data)
		if err != nil || emitted || action != (control.InputAction{}) {
			t.Fatalf("message=%#x data=%+v action=%+v emitted=%t error=%v", test.message, test.data, action, emitted, err)
		}
	}
}

func TestObservedHookMouseWheelCountsPhysicalAndMappedStages(t *testing.T) {
	observer := &recordingSourcePipelineObserver{}
	physical := mouseHookData{MouseData: uint32(uint16(120)) << 16}
	if _, emitted, err := mapObservedHookMouseWheel(observer, wmMouseWheel, physical); err != nil || !emitted {
		t.Fatalf("physical wheel emitted=%t error=%v", emitted, err)
	}
	if _, emitted, err := mapObservedHookMouseWheel(observer, wmMouseWheel, mouseHookData{}); err != nil || emitted {
		t.Fatalf("zero wheel emitted=%t error=%v", emitted, err)
	}
	synthesized := physical
	synthesized.Flags = llmhfInjected
	if _, emitted, err := mapObservedHookMouseWheel(observer, wmMouseWheel, synthesized); err != nil || !emitted {
		t.Fatalf("synthesized wheel emitted=%t error=%v", emitted, err)
	}
	tagged := synthesized
	tagged.ExtraInfo = krasisInjectedExtraInfo
	if _, emitted, err := mapObservedHookMouseWheel(observer, wmMouseWheel, tagged); err != nil || emitted {
		t.Fatalf("tagged wheel emitted=%t error=%v", emitted, err)
	}
	if observer.hookObserved != 2 || observer.synthesizedObserved != 1 || observer.mapped != 2 {
		t.Fatalf("pipeline observer = %+v", observer)
	}
}

func TestSourceWindowWheelOwnershipPassesPhysicalAndSuppressesTaggedInput(t *testing.T) {
	inactive := &stubSourceActivation{}
	if owned, suppress := sourceWindowOwnsHookWheel(inactive, mouseHookData{}); owned || suppress {
		t.Fatalf("inactive ownership = %t/%t", owned, suppress)
	}
	active := &stubSourceActivation{active: true, ownsWheel: true}
	if owned, suppress := sourceWindowOwnsHookWheel(active, mouseHookData{}); !owned || suppress {
		t.Fatalf("physical ownership = %t/%t", owned, suppress)
	}
	if owned, suppress := sourceWindowOwnsHookWheel(active, mouseHookData{Flags: llmhfInjected}); !owned || suppress {
		t.Fatalf("untagged synthesized ownership = %t/%t", owned, suppress)
	}
	if owned, suppress := sourceWindowOwnsHookWheel(active, mouseHookData{ExtraInfo: krasisInjectedExtraInfo}); !owned || !suppress {
		t.Fatalf("tagged ownership = %t/%t", owned, suppress)
	}
	active.ownsWheel = false
	if owned, suppress := sourceWindowOwnsHookWheel(active, mouseHookData{}); owned || suppress {
		t.Fatalf("foreground-lost ownership = %t/%t", owned, suppress)
	}
}

func TestActiveRawWheelObservationIsDiagnosticOnly(t *testing.T) {
	observer := &recordingSourcePipelineObserver{}
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1)}
	session := startObservedSourceSession(t, 1, sink, observer)
	mouse := rawMouse{Buttons: uint32(rawMouseWheel|rawMouseHWheel) | uint32(uint16(120))<<16}
	if err := routeRawMouse(session, nil, newSourcePointerState(point{X: 0}), rawInputHeader{Device: 0}, mouse); err != nil {
		t.Fatal(err)
	}
	if observer.rawObserved != 2 {
		t.Fatalf("raw wheel observations = %d, want 2", observer.rawObserved)
	}
	select {
	case action := <-sink.sent:
		t.Fatalf("raw wheel unexpectedly produced payload action %+v", action)
	default:
	}
}

type stubSourceActivation struct {
	active    bool
	ownsWheel bool
}

func (activation *stubSourceActivation) Activate() error {
	activation.active = true
	activation.ownsWheel = true
	return nil
}

func (activation *stubSourceActivation) Deactivate()     { activation.active = false }
func (activation *stubSourceActivation) Active() bool    { return activation.active }
func (activation *stubSourceActivation) OwnsWheel() bool { return activation.ownsWheel }

func TestRawWheelObservationRequiresActiveUntaggedNonzeroInput(t *testing.T) {
	observer := &recordingSourcePipelineObserver{}
	inactive, err := NewSourceSessionObserved(1, &recordingSourceSink{}, observer)
	if err != nil {
		t.Fatal(err)
	}
	valid := rawMouse{Buttons: uint32(rawMouseWheel) | uint32(uint16(120))<<16}
	observeActiveRawMouseWheel(inactive, valid)
	active := startObservedSourceSession(t, 1, &recordingSourceSink{}, observer)
	observeActiveRawMouseWheel(active, rawMouse{Buttons: rawMouseWheel})
	observeActiveRawMouseWheel(active, rawMouse{
		Buttons: uint32(rawMouseWheel) | uint32(uint16(120))<<16, ExtraInformation: uint32(krasisInjectedExtraInfo),
	})
	observeActiveRawMouseWheel(active, valid)
	if observer.rawObserved != 1 {
		t.Fatalf("raw wheel observations = %d, want 1", observer.rawObserved)
	}
}

func TestActivePhysicalHookHealthExcludesWheelAndInjectedInput(t *testing.T) {
	observer := &recordingSourcePipelineObserver{}
	observeActivePhysicalHookMouse(observer, wmMouseMove, false)
	observeActivePhysicalHookMouse(observer, wmMouseMove, true)
	observeActivePhysicalHookMouse(observer, wmLButtonDown, true)
	observeActivePhysicalHookMouse(observer, wmRButtonUp, true)
	observeActivePhysicalHookMouse(observer, wmMouseWheel, true)
	if observer.activeHookMoves != 1 || observer.activeHookButtons != 2 {
		t.Fatalf("active hook health = moves %d buttons %d", observer.activeHookMoves, observer.activeHookButtons)
	}
}

func TestHookWheelSuppressesOnlyAcceptedRemoteAction(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1)}
	session := startSourceSession(t, 1, sink)
	data := mouseHookData{Point: point{X: 0, Y: 400}, MouseData: uint32(uint16(120)) << 16}
	if !routeMappedPhysicalAction(session, nil, func() (control.InputAction, bool, int32, error) {
		action, emitted, err := mapHookMouseWheel(wmMouseWheel, data)
		return action, emitted, data.Point.X, err
	}) {
		t.Fatal("accepted hook wheel was not suppressed")
	}
	want := control.InputAction{Kind: control.InputWheel, DeltaY: 120}
	if got := receiveAction(t, sink.sent); got != want {
		t.Fatalf("sent action = %+v, want %+v", got, want)
	}

	inactive := newInactiveSourceSession(t)
	if routeMappedPhysicalAction(inactive, nil, func() (control.InputAction, bool, int32, error) {
		action, emitted, err := mapHookMouseWheel(wmMouseWheel, data)
		return action, emitted, data.Point.X, err
	}) {
		t.Fatal("inactive hook wheel was suppressed")
	}
}

func TestHookSynthesizedWheelSuppressesOnlyAcceptedRemoteAction(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1)}
	session := startSourceSession(t, 1, sink)
	data := mouseHookData{
		Point: point{X: 0, Y: 400}, MouseData: uint32(uint16(120)) << 16, Flags: llmhfInjected,
	}
	if !routeMappedPhysicalAction(session, nil, func() (control.InputAction, bool, int32, error) {
		action, emitted, err := mapHookMouseWheel(wmMouseWheel, data)
		return action, emitted, data.Point.X, err
	}) {
		t.Fatal("accepted synthesized hook wheel was not suppressed")
	}
	if got := receiveAction(t, sink.sent); got != (control.InputAction{Kind: control.InputWheel, DeltaY: 120}) {
		t.Fatalf("sent synthesized wheel = %+v", got)
	}
}

func TestMapRawMouseDoesNotAllocatePerPacket(t *testing.T) {
	mouse := rawMouse{LastX: 3, Buttons: rawMouseLeftButtonDown}
	allocations := testing.AllocsPerRun(1000, func() {
		actions, err := mapRawMouse(mouse)
		if err != nil || actions.count != 2 {
			panic("unexpected raw mouse mapping result")
		}
	})
	if allocations != 0 {
		t.Fatalf("mapRawMouse allocations = %f, want 0", allocations)
	}
}

func TestRouteRawMouseSendsExactlyOneActionWhileActive(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 2)}
	session := startSourceSession(t, 2, sink)
	state := newSourcePointerState(point{X: 1535, Y: 400})
	if err := routeRawMouse(session, nil, state, rawInputHeader{Device: 1}, rawMouse{LastX: 8, LastY: -2}); err != nil {
		t.Fatal(err)
	}
	want := control.InputAction{Kind: control.InputPointerMove, DeltaX: 8, DeltaY: -2}
	if got := receiveAction(t, sink.sent); got != want {
		t.Fatalf("sent action = %+v, want %+v", got, want)
	}
	select {
	case duplicate := <-sink.sent:
		t.Fatalf("duplicate pointer action = %+v", duplicate)
	default:
	}
}

func TestPhysicalHookMouseInputSuppressesOnlyWhileActive(t *testing.T) {
	session := newInactiveSourceSession(t)
	physical := mouseHookData{Point: point{X: 10, Y: 10}}
	if shouldSuppressPhysicalMouseInput(session, physical) {
		t.Fatal("inactive session suppressed physical mouse input")
	}

	active := startSourceSession(t, 1, &recordingSourceSink{})
	if !shouldSuppressPhysicalMouseInput(active, physical) {
		t.Fatal("active session did not suppress physical mouse input")
	}
	if !shouldSuppressPhysicalMouseInput(active, mouseHookData{MouseData: 1}) {
		t.Fatal("active session did not suppress physical mouse button input")
	}
	if shouldSuppressPhysicalMouseInput(active, mouseHookData{Flags: llmhfInjected}) {
		t.Fatal("active session suppressed injected mouse input")
	}
	if shouldSuppressPhysicalMouseInput(active, mouseHookData{ExtraInfo: krasisInjectedExtraInfo}) {
		t.Fatal("active session suppressed Krasis-tagged mouse input")
	}
}

func TestSourcePointerStateTracksInjectedCursorPositionForBoundaryOnly(t *testing.T) {
	state := newSourcePointerState(point{X: 100, Y: 20})
	state.observe(point{X: 400, Y: 30})
	x, known := state.pointerX()
	if !known || x != 400 {
		t.Fatalf("pointer state x=%d known=%t", x, known)
	}
}
