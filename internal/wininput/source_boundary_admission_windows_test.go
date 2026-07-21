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
	t.Cleanup(func() {
		sourceForegroundGUIThreadInfo = originalInfo
		sourceAnyInputPressed = originalPressed
	})

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

func TestSourceBoundaryAdmissionFailsClosed(t *testing.T) {
	original := sourceForegroundGUIThreadInfo
	t.Cleanup(func() { sourceForegroundGUIThreadInfo = original })
	cause := errors.New("query failed")
	sourceForegroundGUIThreadInfo = func(*sourceGUIThreadInfo) error { return cause }

	decision, err := (&SourceHook{}).AdmitBoundary(
		context.Background(),
		control.BoundaryIntent{Edge: control.ScreenEdgeRight},
	)
	if decision != 0 || !errors.Is(err, cause) {
		t.Fatalf("AdmitBoundary() = %v, %v", decision, err)
	}
}
