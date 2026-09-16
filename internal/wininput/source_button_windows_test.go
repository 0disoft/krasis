//go:build windows

package wininput

import (
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

func TestHookButtonsOwnTransitionsAndIgnoreInjection(t *testing.T) {
	for _, pair := range [][3]uint32{{wmLButtonDown, wmLButtonUp, MouseButtonLeft}, {wmRButtonDown, wmRButtonUp, MouseButtonRight}, {wmMButtonDown, wmMButtonUp, MouseButtonMiddle}, {wmXButtonDown, wmXButtonUp, MouseButtonX1}} {
		for i, kind := range []control.InputActionKind{control.InputButtonDown, control.InputButtonUp} {
			action, ok := mapSourceHookButton(pair[i], mouseHookData{MouseData: 1 << 16})
			if !ok || action.Kind != kind || action.Code != pair[2] {
				t.Fatalf("wrong mapping: %+v", action)
			}
			if _, ok := mapSourceHookButton(pair[i], mouseHookData{Flags: llmhfInjected}); ok {
				t.Fatal("injected input tracked")
			}
		}
	}
}

func TestRawButtonCannotReplayAfterGrant(t *testing.T) {
	session, _ := NewSourceSession(2, &recordingSourceSink{})
	session.started.Store(true)
	if err := session.Activate(); err != nil {
		t.Fatal(err)
	}
	if err := routeRawMouse(session, nil, newSourcePointerState(point{}), rawInputHeader{}, rawMouse{Buttons: rawMouseLeftButtonDown | rawMouseLeftButtonUp}); err != nil {
		t.Fatal(err)
	}
	if len(session.queue) != 0 {
		t.Fatal("delayed raw buttons entered new lease")
	}
}
