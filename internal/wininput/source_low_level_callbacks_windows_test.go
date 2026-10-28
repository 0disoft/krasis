//go:build windows

package wininput

import (
	"errors"
	"testing"
	"unsafe"
)

func TestIgnoredSourceCallbacksSkipDesktopWorkButPreservePointerAndPresentation(t *testing.T) {
	for _, tag := range []uintptr{krasisInjectedExtraInfo, selfTestExtraInfo} {
		session := newInactiveSourceSession(t)
		boundary := &recordingBoundaryObserver{}
		pointer := newSourcePointerState(point{X: 10})
		shield := &returnCursorShield{}
		checked, continued := 0, 0
		callbacks := newSourceLowLevelCallbacks(session, boundary, nil, nil, pointer, nil, shield,
			func() error { checked++; return errors.New("inactive desktop") },
			func(int, uintptr, uintptr) uintptr { continued++; return 42 })
		// Ignored input must not acquire the physical-input gate, emit a
		// boundary intent, or abort capture on an unrelated desktop query.
		session.physicalMu.Lock()
		key := keyboardHookData{ExtraInfo: tag}
		mouse := mouseHookData{ExtraInfo: tag, Point: point{X: 90}}
		keyboardResult := callbacks.keyboard(hcAction, wmKeyDown, uintptr(unsafe.Pointer(&key)))
		mouseResult := callbacks.mouse(hcAction, wmMouseMove, uintptr(unsafe.Pointer(&mouse)))
		session.physicalMu.Unlock()
		if keyboardResult != 42 || mouseResult != 42 || continued != 2 || checked != 0 || boundary.calls != 0 || session.terminal.Load() {
			t.Fatal("ignored input performed physical capture work", keyboardResult, mouseResult, checked, boundary.calls)
		}
		if x, known := pointer.pointerX(); !known || x != 90 || shield.generation.Load() != 2 {
			t.Fatal("injected cursor position or pending presentation invalidation lost", x, known, shield.generation.Load())
		}
	}
}

func TestSourceCallbacksKeepPhysicalGuardAndSynthesizedWheelRouting(t *testing.T) {
	session := newInactiveSourceSession(t)
	boundary := &recordingBoundaryObserver{}
	checked, continued := 0, 0
	callbacks := newSourceLowLevelCallbacks(session, boundary, nil, nil, nil, nil, nil,
		func() error { checked++; return nil },
		func(int, uintptr, uintptr) uintptr { continued++; return 42 })
	key := keyboardHookData{ScanCode: 1}
	callbacks.keyboard(hcAction, wmKeyDown, uintptr(unsafe.Pointer(&key)))
	wheel := mouseHookData{Flags: llmhfInjected, MouseData: uint32(120) << 16}
	callbacks.mouse(hcAction, wmMouseWheel, uintptr(unsafe.Pointer(&wheel)))
	if checked != 2 || boundary.calls != 2 || continued != 2 {
		t.Fatal("physical guard or precision touchpad wheel routing changed", checked, boundary.calls, continued)
	}
	// The window owns suppression of tagged synthetic wheels. Do not turn
	// these into an ordinary pass-through and duplicate scroll delivery.
	callbacks = newSourceLowLevelCallbacks(session, boundary, nil,
		&stubSourceActivation{active: true, ownsWheel: true}, nil, nil, nil,
		func() error { checked++; return nil },
		func(int, uintptr, uintptr) uintptr { continued++; return 42 })
	wheel.ExtraInfo = krasisInjectedExtraInfo
	if callbacks.mouse(hcAction, wmMouseWheel, uintptr(unsafe.Pointer(&wheel))) != 1 || continued != 2 || boundary.calls != 2 {
		t.Fatal("owned wheel suppression changed")
	}
	// OS-injected keyboard input was already excluded by the mapper.
	key.Flags = llkhfInjected
	if callbacks.keyboard(hcAction, wmKeyDown, uintptr(unsafe.Pointer(&key))) != 42 || checked != 3 || boundary.calls != 2 {
		t.Fatal("unmapped injected keyboard performed physical work")
	}
}

func TestIgnoredSourceInputChecksVisibleShieldAndRetainsFailures(t *testing.T) {
	oldHide := returnCursorHideWindow
	t.Cleanup(func() { returnCursorHideWindow = oldHide })
	hidden, checked := 0, 0
	returnCursorHideWindow = func(uintptr) error { hidden++; return nil }
	shield := &returnCursorShield{visible: true}
	failure := errors.New("inactive desktop")
	if err := shield.beforeIgnoredInput(func() error { checked++; return failure }); !errors.Is(err, failure) || hidden != 0 || !shield.visible {
		t.Fatal("visible shield bypassed desktop validation", err, hidden)
	}
	if err := shield.beforeIgnoredInput(func() error { checked++; return nil }); err != nil || hidden != 1 || shield.visible || checked != 2 {
		t.Fatal("visible shield was not restored before input", err, hidden, checked)
	}
	shield.record("cursor-restore", failure)
	if err := shield.beforeIgnoredInput(func() error { t.Fatal("hidden shield queried desktop"); return nil }); !errors.Is(err, failure) {
		t.Fatal("earlier cleanup failure was dropped", err)
	}
}

func TestSourceMouseProgressCannotBeRenewedByInjectedTraffic(t *testing.T) {
	session := newInactiveSourceSession(t)
	pointer := newSourcePointerState(point{})
	pointer.progress = &sourceHookProgress{}
	callbacks := newSourceLowLevelCallbacks(session, nil, nil, nil, pointer, nil, nil,
		func() error { return nil }, func(int, uintptr, uintptr) uintptr { return 42 })
	for _, data := range []mouseHookData{
		{ExtraInfo: krasisInjectedExtraInfo}, {ExtraInfo: selfTestExtraInfo}, {Flags: llmhfInjected},
	} {
		callbacks.mouse(hcAction, wmMouseMove, uintptr(unsafe.Pointer(&data)))
	}
	if pointer.progress.mouse.Load() != 0 {
		t.Fatal("injected traffic hid missing physical hook progress")
	}
	data := mouseHookData{}
	callbacks.mouse(hcAction, wmMouseMove, uintptr(unsafe.Pointer(&data)))
	if pointer.progress.mouse.Load() != 1 {
		t.Fatal("physical hook progress not observed")
	}
}

func TestReturnCursorRepeatedBoundaryMouseReportsStayHidden(t *testing.T) {
	oldHide := returnCursorHideWindow
	t.Cleanup(func() { returnCursorHideWindow = oldHide })
	for _, flags := range []uint32{0, llmhfInjected} {
		for _, tag := range []uintptr{0, krasisInjectedExtraInfo, selfTestExtraInfo} {
			session := newInactiveSourceSession(t)
			position := point{X: 0, Y: 400}
			shield := &returnCursorShield{visible: true, position: position}
			pointer := newSourcePointerState(position)
			hidden, checked, continued := 0, 0, 0
			returnCursorHideWindow = func(uintptr) error { hidden++; return nil }
			callbacks := newSourceLowLevelCallbacks(session, nil, nil, nil, pointer, nil, shield,
				func() error { checked++; return nil },
				func(int, uintptr, uintptr) uintptr { continued++; return 42 })
			mouse := mouseHookData{Point: position, Flags: flags, ExtraInfo: tag}
			for i := 0; i < 3; i++ {
				if callbacks.mouse(hcAction, wmMouseMove, uintptr(unsafe.Pointer(&mouse))) != 42 {
					t.Fatal("stationary input was swallowed")
				}
			}
			wantChecks := 3
			if tag != 0 {
				wantChecks = 0 // No native cursor operation for a tagged stationary report.
			}
			if hidden != 0 || !shield.visible || shield.generation.Load() != 0 || checked != wantChecks {
				t.Fatal("stationary edge reports revealed cursor or performed unnecessary desktop work")
			}
			mouse.Point.X++
			callbacks.mouse(hcAction, wmMouseMove, uintptr(unsafe.Pointer(&mouse)))
			if hidden != 1 || shield.visible || shield.generation.Load() != 1 || continued != 4 {
				t.Fatal("actual movement did not restore cursor before hook continuation")
			}
			shield.visible, mouse.Point = true, position
			callbacks.mouse(hcAction, wmLButtonDown, uintptr(unsafe.Pointer(&mouse)))
			if hidden != 2 || shield.visible {
				t.Fatal("click at unchanged coordinates did not restore cursor")
			}
		}
	}
}
