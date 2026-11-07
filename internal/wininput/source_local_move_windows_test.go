//go:build windows

package wininput

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestInactivePhysicalMovesDoNotSubmitAuthorityQueries(t *testing.T) {
	session := newInactiveSourceSession(t)
	pointer := newSourcePointerState(point{})
	pointer.progress = &sourceHookProgress{}
	boundary := &recordingBoundaryObserver{}
	checks, continued := 0, 0
	callbacks := newSourceLowLevelCallbacks(session, boundary, nil, nil, pointer, nil, nil,
		func() error { checks++; return nil },
		func(int, uintptr, uintptr) uintptr { continued++; return 42 })
	for i := int32(1); i <= 1000; i++ {
		payload := nativeHookPayload(t, mouseHookData{Point: point{X: i}})
		if callbacks.mouse(hcAction, wmMouseMove, payload) != 42 {
			t.Fatal("local movement was suppressed")
		}
	}
	if x, known := pointer.pointerX(); !known || x != 1000 || checks != 0 || continued != 1000 || boundary.calls != 0 || pointer.progress.mouse.Load() != 1000 {
		t.Fatal("local movement queried authority, emitted input or lost physical progress", x, checks, continued, boundary.calls)
	}
	// The next active movement must still take a fresh check and be suppressed.
	session.active.Store(true)
	if callbacks.mouse(hcAction, wmMouseMove, nativeHookPayload(t, mouseHookData{})) != 1 || checks != 1 || continued != 1000 {
		t.Fatal("local fast path escaped into active capture", checks, continued)
	}
}

func TestLocalMovePreservesActivationAndPresentationGuards(t *testing.T) {
	for _, mode := range []string{"active", "gate-busy", "visible"} {
		t.Run(mode, func(t *testing.T) {
			session := newInactiveSourceSession(t)
			pointer := newSourcePointerState(point{})
			shield := &returnCursorShield{}
			switch mode {
			case "active":
				session.active.Store(true)
			case "gate-busy":
				session.physicalMu.Lock()
				defer session.physicalMu.Unlock()
			case "visible":
				shield.visible = true
			}
			checks := 0
			failure := errors.New("authority rejected")
			callbacks := newSourceLowLevelCallbacks(session, nil, nil, nil, pointer, nil, shield,
				func() error { checks++; return failure }, func(int, uintptr, uintptr) uintptr { return 42 })
			if callbacks.mouse(hcAction, wmMouseMove, nativeHookPayload(t, mouseHookData{})) != 42 || checks != 1 || !session.terminal.Load() {
				t.Fatal("active, contended or native presentation work bypassed denial", checks)
			}
		})
	}
}

func TestLocalMoveRetainsPendingCursorAndCleanupFailures(t *testing.T) {
	position := point{X: 0, Y: 400}
	session := newInactiveSourceSession(t)
	pointer := newSourcePointerState(point{})
	shield := &returnCursorShield{}
	shield.generation.Store(7)
	shield.snapshotRequest.Store(&returnCursorSnapshotRequest{valid: true, generation: 7, position: position})
	if local, err := observeInactiveHookMove(session, pointer, shield, position); !local || err != nil || shield.generation.Load() != 7 {
		t.Fatal("stationary local report invalidated pending cursor", local, err)
	}
	if local, err := observeInactiveHookMove(session, pointer, shield, point{X: 1}); !local || err != nil || shield.generation.Load() != 8 {
		t.Fatal("real local movement did not invalidate pending cursor", local, err)
	}
	failure := errors.New("earlier native cleanup failed")
	shield.record("cursor-restore", failure)
	if local, err := observeInactiveHookMove(session, pointer, shield, point{X: 2}); !local || !errors.Is(err, failure) {
		t.Fatal("local movement hid native cleanup failure", local, err)
	}
	if x, _ := pointer.pointerX(); x != 1 {
		t.Fatal("failed local movement changed observed coordinates", x)
	}
}

func TestLocalMoveCannotBypassAnActivationOwningThePhysicalGate(t *testing.T) {
	session := newInactiveSourceSession(t)
	pointer := newSourcePointerState(point{})
	session.physicalMu.Lock()
	defer session.physicalMu.Unlock()
	checks := 0
	callbacks := newSourceLowLevelCallbacks(session, nil, nil, nil, pointer, nil, nil,
		func() error { checks++; session.active.Store(true); return nil },
		func(int, uintptr, uintptr) uintptr { t.Error("active movement passed through"); return 42 })
	if callbacks.mouse(hcAction, wmMouseMove, nativeHookPayload(t, mouseHookData{})) != 1 || checks != 1 {
		t.Fatal("contended activation bypassed fresh proof or suppression", checks)
	}
}

func TestLocalHookObservationDoesNotForwardOrDuplicateRawMovement(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 2)}
	session := startSourceSession(t, 4, sink)
	session.Deactivate()
	pointer := newSourcePointerState(point{})
	checks := 0
	callbacks := newSourceLowLevelCallbacks(session, nil, nil, nil, pointer, nil, nil,
		func() error { checks++; return nil }, func(int, uintptr, uintptr) uintptr { return 42 })
	payload := nativeHookPayload(t, mouseHookData{Point: point{X: 80}})
	if callbacks.mouse(hcAction, wmMouseMove, payload) != 42 || checks != 0 {
		t.Fatal("local hook performed capture work")
	}
	if err := routeRawMouse(session, nil, pointer, rawInputHeader{Device: 1}, rawMouse{LastX: 8}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sink.sent:
		t.Fatal("inactive observation forwarded movement")
	default:
	}
	if err := session.Activate(); err != nil {
		t.Fatal(err)
	}
	if callbacks.mouse(hcAction, wmMouseMove, payload) != 1 || checks != 1 {
		t.Fatal("active hook skipped fresh authorization or suppression")
	}
	if err := routeRawMouse(session, nil, pointer, rawInputHeader{Device: 1}, rawMouse{LastX: 9}); err != nil {
		t.Fatal(err)
	}
	if action := receiveAction(t, sink.sent); action.Kind != control.InputPointerMove || action.DeltaX != 9 {
		t.Fatal("new active movement changed", action)
	}
	select {
	case action := <-sink.sent:
		t.Fatal("low-level observation duplicated raw movement", action)
	default:
	}
}
