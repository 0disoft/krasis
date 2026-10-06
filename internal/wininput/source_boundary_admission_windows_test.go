//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"unsafe"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
)

func TestSourceBoundaryAdmissionClassifiesForegroundInteraction(t *testing.T) {
	originalInfo := sourceForegroundGUIThreadInfo
	originalPressed := sourceAnyInputPressed
	originalCancel := sourceCancelMenu
	t.Cleanup(func() {
		sourceForegroundGUIThreadInfo = originalInfo
		sourceAnyInputPressed = originalPressed
		sourceCancelMenu = originalCancel
	})
	sourceCancelMenu = func(context.Context, uintptr) bool { return false }

	for _, test := range []struct {
		name     string
		flags    uint32
		capture  uintptr
		moveSize uintptr
		pressed  bool
		want     peer.BoundaryAdmissionDecision
	}{
		{name: "allowed", want: peer.BoundaryAdmissionAllowed},
		{name: "menu mode", flags: guiInMenuMode, want: peer.BoundaryAdmissionBlockedLocalMenu},
		{name: "system menu", flags: guiSystemMenuMode, want: peer.BoundaryAdmissionBlockedLocalMenu},
		{name: "popup menu", flags: guiPopupMenuMode, want: peer.BoundaryAdmissionBlockedLocalMenu},
		{name: "mouse capture", capture: 42, want: peer.BoundaryAdmissionBlockedMouseCapture},
		{name: "move size flag", flags: guiInMoveSize, want: peer.BoundaryAdmissionBlockedMoveSize},
		{name: "move size window", moveSize: 43, want: peer.BoundaryAdmissionBlockedMoveSize},
		{name: "pressed input", pressed: true, want: peer.BoundaryAdmissionBlockedPressedInput},
		{
			name: "menu has priority over other local interaction", flags: guiInMenuMode,
			capture: 42, moveSize: 43, pressed: true, want: peer.BoundaryAdmissionBlockedLocalMenu,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sourceForegroundGUIThreadInfo = func(info *sourceGUIThreadInfo) error {
				if info.Size != uint32(unsafe.Sizeof(sourceGUIThreadInfo{})) {
					t.Fatalf("GUITHREADINFO size = %d", info.Size)
				}
				info.Flags = test.flags
				info.Capture = test.capture
				info.MoveSize = test.moveSize
				return nil
			}
			sourceAnyInputPressed = func() bool { return test.pressed }
			decision, err := (&SourceHook{}).AdmitBoundary(
				context.Background(),
				control.BoundaryIntent{Edge: control.ScreenEdgeLeft},
			)
			if err != nil {
				t.Fatal(err)
			}
			if decision != test.want {
				t.Fatalf("AdmitBoundary() = %v, want %v", decision, test.want)
			}
		})
	}
}

func TestBoundaryDismissesIdleMouseCaptureBeforeCrossing(t *testing.T) {
	oldInfo, oldPressed, oldCancel := sourceForegroundGUIThreadInfo, sourceAnyInputPressed, sourceCancelMenu
	oldPopup := sourceIsContextPopup
	sourceIsContextPopup = func(uintptr, uintptr) bool { return true }
	t.Cleanup(func() {
		sourceForegroundGUIThreadInfo, sourceAnyInputPressed, sourceCancelMenu = oldInfo, oldPressed, oldCancel
		sourceIsContextPopup = oldPopup
	})
	for _, test := range []struct {
		name      string
		delivered bool
		held      bool
		after     sourceGUIThreadInfo
		want      peer.BoundaryAdmissionDecision
	}{
		{"released", true, false, sourceGUIThreadInfo{Active: 7}, peer.BoundaryAdmissionAllowed},
		{"message rejected", false, false, sourceGUIThreadInfo{}, peer.BoundaryAdmissionBlockedMouseCapture},
		{"button held", true, true, sourceGUIThreadInfo{}, peer.BoundaryAdmissionBlockedMouseCapture},
		{"capture retained", true, false, sourceGUIThreadInfo{Active: 7, Capture: 42}, peer.BoundaryAdmissionBlockedMouseCapture},
		{"capture moved", true, false, sourceGUIThreadInfo{Active: 7, Capture: 43}, peer.BoundaryAdmissionBlockedMouseCapture},
		{"foreground changed", true, false, sourceGUIThreadInfo{Active: 8}, peer.BoundaryAdmissionBlockedMouseCapture},
		{"menu appeared", true, false, sourceGUIThreadInfo{Active: 7, Flags: guiInMenuMode}, peer.BoundaryAdmissionBlockedMouseCapture},
	} {
		t.Run(test.name, func(t *testing.T) {
			queries, sends := 0, 0
			sourceForegroundGUIThreadInfo = func(info *sourceGUIThreadInfo) error {
				queries++
				if queries == 1 {
					*info = sourceGUIThreadInfo{Active: 7, Capture: 42}
				} else {
					*info = test.after
				}
				return nil
			}
			sourceAnyInputPressed = func() bool { return test.held }
			sourceCancelMenu = func(_ context.Context, owner uintptr) bool {
				sends++
				if owner != 42 {
					t.Fatalf("cancel target = %d, want capture owner 42", owner)
				}
				return test.delivered
			}
			hook := &SourceHook{}
			hook.contextMenu.Store(7)
			decision, err := hook.AdmitBoundary(context.Background(), control.BoundaryIntent{Edge: control.ScreenEdgeRight})
			if err != nil || decision != test.want || (test.held && sends != 0) {
				t.Fatalf("decision=%v err=%v sends=%d", decision, err, sends)
			}
		})
	}
}

func TestBoundaryDoesNotCancelUnknownOrNonPopupCapture(t *testing.T) {
	oldInfo, oldPressed, oldCancel, oldPopup := sourceForegroundGUIThreadInfo, sourceAnyInputPressed, sourceCancelMenu, sourceIsContextPopup
	t.Cleanup(func() {
		sourceForegroundGUIThreadInfo, sourceAnyInputPressed, sourceCancelMenu, sourceIsContextPopup = oldInfo, oldPressed, oldCancel, oldPopup
	})
	sourceForegroundGUIThreadInfo = func(info *sourceGUIThreadInfo) error { *info = sourceGUIThreadInfo{Active: 7, Capture: 42}; return nil }
	sourceAnyInputPressed = func() bool { return false }
	sourceCancelMenu = func(context.Context, uintptr) bool { t.Fatal("unrelated capture canceled"); return true }
	for _, test := range []struct {
		gesture uintptr
		popup   bool
	}{{0, true}, {8, true}, {7, false}} {
		sourceIsContextPopup = func(uintptr, uintptr) bool { return test.popup }
		hook := &SourceHook{}
		hook.contextMenu.Store(test.gesture)
		decision, err := hook.AdmitBoundary(context.Background(), control.BoundaryIntent{})
		if err != nil || decision != peer.BoundaryAdmissionBlockedMouseCapture {
			t.Fatal(decision, err)
		}
	}
}

func TestSourceBoundaryAdmissionFailsClosed(t *testing.T) {
	original := sourceForegroundGUIThreadInfo
	t.Cleanup(func() { sourceForegroundGUIThreadInfo = original })
	cause := errors.New("query failed")
	sourceForegroundGUIThreadInfo = func(*sourceGUIThreadInfo) error { return cause }

	decision, err := (&SourceHook{}).AdmitBoundary(
		context.Background(),
		control.BoundaryIntent{Edge: control.ScreenEdgeRight},
	)
	if decision != 0 || !errors.Is(err, cause) || !errors.Is(err, peer.ErrBoundaryInspectionUnavailable) {
		t.Fatalf("AdmitBoundary() = %v, %v", decision, err)
	}
}

func TestBoundaryInspectionCancellationAndGuardRemainFatal(t *testing.T) {
	old := sourceForegroundGUIThreadInfo
	t.Cleanup(func() { sourceForegroundGUIThreadInfo = old })
	cause := errors.New("guard failed")
	sourceForegroundGUIThreadInfo = func(*sourceGUIThreadInfo) error { t.Fatal("failed guard inspected GUI"); return nil }
	_, err := (&SourceHook{boundaryGuard: func() error { return cause }}).AdmitBoundary(context.Background(), control.BoundaryIntent{})
	if !errors.Is(err, cause) || errors.Is(err, peer.ErrBoundaryInspectionUnavailable) {
		t.Fatal("guard failure became a recoverable inspection", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	sourceForegroundGUIThreadInfo = func(*sourceGUIThreadInfo) error { cancel(); return errors.New("query failed") }
	_, err = (&SourceHook{}).AdmitBoundary(ctx, control.BoundaryIntent{})
	if !errors.Is(err, context.Canceled) || errors.Is(err, peer.ErrBoundaryInspectionUnavailable) {
		t.Fatal("cancelled inspection continued the session", err)
	}
}
func TestBoundaryDismissesMenuAndRechecksInteraction(t *testing.T) {
	oldInfo, oldPressed, oldCancel := sourceForegroundGUIThreadInfo, sourceAnyInputPressed, sourceCancelMenu
	t.Cleanup(func() {
		sourceForegroundGUIThreadInfo, sourceAnyInputPressed, sourceCancelMenu = oldInfo, oldPressed, oldCancel
	})
	for _, test := range []struct {
		name            string
		delivered, held bool
		after           sourceGUIThreadInfo
		want            peer.BoundaryAdmissionDecision
	}{
		{"closed", true, false, sourceGUIThreadInfo{Active: 7}, peer.BoundaryAdmissionAllowed},
		{"hung menu", false, false, sourceGUIThreadInfo{}, peer.BoundaryAdmissionBlockedLocalMenu},
		{"held button", true, true, sourceGUIThreadInfo{}, peer.BoundaryAdmissionBlockedLocalMenu},
		{"foreground changed", true, false, sourceGUIThreadInfo{Active: 8}, peer.BoundaryAdmissionBlockedLocalMenu},
		{"capture remains", true, false, sourceGUIThreadInfo{Active: 7, Capture: 9}, peer.BoundaryAdmissionBlockedMouseCapture},
		{"move size remains", true, false, sourceGUIThreadInfo{Active: 7, Flags: guiInMoveSize}, peer.BoundaryAdmissionBlockedMoveSize},
		{"different menu", true, false, sourceGUIThreadInfo{Active: 7, MenuOwner: 8, Flags: guiInMenuMode}, peer.BoundaryAdmissionBlockedLocalMenu},
		{"menu remains open", true, false, sourceGUIThreadInfo{Active: 7, MenuOwner: 42, Flags: guiInMenuMode}, peer.BoundaryAdmissionBlockedLocalMenu},
	} {
		t.Run(test.name, func(t *testing.T) {
			queries, sends := 0, 0
			sourceForegroundGUIThreadInfo = func(info *sourceGUIThreadInfo) error {
				queries++
				if queries == 1 {
					*info = sourceGUIThreadInfo{Active: 7, MenuOwner: 42, Flags: guiSystemMenuMode}
				} else {
					*info = test.after
				}
				return nil
			}
			sourceAnyInputPressed = func() bool { return test.held }
			sourceCancelMenu = func(_ context.Context, owner uintptr) bool {
				sends++
				if owner == 9 && test.name == "capture remains" {
					return false
				}
				if owner != 42 {
					t.Fatal("wrong menu owner")
				}
				return test.delivered
			}
			decision, err := (&SourceHook{}).AdmitBoundary(context.Background(), control.BoundaryIntent{Edge: control.ScreenEdgeRight})
			if err != nil || decision != test.want || (test.held && sends != 0) {
				t.Fatalf("decision=%v err=%v sends=%d", decision, err, sends)
			}
		})
	}
}
