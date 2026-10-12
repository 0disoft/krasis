//go:build windows

package wininput

import (
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
)

func TestBoundaryPressedEvidenceSeparatesNativeAndPhysicalLedger(t *testing.T) {
	oldGUI, oldPressed := sourceForegroundGUIThreadInfo, sourceAnyInputPressed
	t.Cleanup(func() { sourceForegroundGUIThreadInfo, sourceAnyInputPressed = oldGUI, oldPressed })
	sourceForegroundGUIThreadInfo = func(*sourceGUIThreadInfo) error { return nil }
	for _, tc := range []struct {
		native, hook bool
		want         string
	}{
		{true, false, "boundary_pressed_native_only"},
		{false, true, "boundary_pressed_hook_only"},
		{true, true, "boundary_pressed_native_and_hook"},
	} {
		sourceAnyInputPressed = func() bool { return tc.native }
		session, _ := NewSourceSession(4, &recordingSourceSink{})
		if tc.hook {
			routePhysicalAction(session, nil, control.InputAction{Kind: control.InputKeyDown, Code: 1}, 0)
		}
		observed := ""
		hook := &SourceHook{admissionReport: func(code string) { observed = code }}
		hook.physicalSession.Store(session)
		decision, err := hook.AdmitBoundary(context.Background(), control.BoundaryIntent{})
		if err != nil || decision != peer.BoundaryAdmissionBlockedPressedInput || observed != tc.want {
			t.Fatal(decision, err, observed, tc.want)
		}
	}
}
