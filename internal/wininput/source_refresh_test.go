package wininput

import (
	"testing"

	"github.com/0disoft/krasis/internal/control"
)

func TestCaptureRefreshFencesGrantsWithoutRetiringSender(t *testing.T) {
	session, _ := NewSourceSession(4, &recordingSourceSink{})
	session.started.Store(true)
	if !session.PauseCapture() || !session.PauseCapture() {
		t.Fatal("idle capture could not pause or coalesce another transition")
	}
	if err := session.Activate(); err == nil || session.terminal.Load() {
		t.Fatal("refresh admitted a grant or retired the sender")
	}
	session.ResumeCapture()
	session.physicalCheck = func() bool { return true }
	if err := session.Activate(); err == nil {
		t.Fatal("refresh bypassed the native pressed-state guard")
	}
	session.physicalCheck = func() bool { return false }
	if err := session.Activate(); err != nil || session.PauseCapture() {
		t.Fatal("live outbound generation could be replaced", err)
	}
}

func TestCaptureRefreshPreservesLocalPressAndReturnOwnership(t *testing.T) {
	session, _ := NewSourceSession(4, &recordingSourceSink{})
	routePhysicalAction(session, nil, control.InputAction{Kind: control.InputKeyDown, Code: 1}, 0)
	if session.PauseCapture() || session.PhysicalPressCount() != 1 {
		t.Fatal("local press was erased or crossed owner replacement")
	}
	routePhysicalAction(session, nil, control.InputAction{Kind: control.InputKeyUp, Code: 1}, 0)
	session.yieldPending = 1
	if session.PauseCapture() {
		t.Fatal("pending local return was ignored")
	}
	session.yieldPending = 0
	session.Retire()
	if session.PauseCapture() {
		t.Fatal("retired sender was reopened")
	}
}

func TestRetiredCaptureRevokesOnceAndKeepsReplacementFenced(t *testing.T) {
	session, _ := NewSourceSession(4, &recordingSourceSink{})
	session.started.Store(true)
	if err := session.Activate(); err != nil {
		t.Fatal(err)
	}
	if !session.PauseRetiredCapture() || session.Active() || !session.LocalReturnPending() {
		t.Fatal("retired native owner kept forwarding or skipped revoke")
	}
	generation := <-session.LocalReturns()
	if !session.PauseRetiredCapture() || len(session.yields) != 0 {
		t.Fatal("retirement replayed the pending revoke")
	}
	session.CompleteLocalReturn(generation)
	if session.LocalReturnPending() || session.Activate() == nil || session.terminal.Load() {
		t.Fatal("revoke reopened capture or retired the reusable sender")
	}
	session.ResumeCapture()
	if err := session.Activate(); err != nil {
		t.Fatal("ready replacement could not accept a fresh grant", err)
	}
}

func TestRetiredCaptureCannotErasePhysicalPressOrReviveTerminalSender(t *testing.T) {
	session, _ := NewSourceSession(4, &recordingSourceSink{})
	routePhysicalAction(session, nil, control.InputAction{Kind: control.InputKeyDown, Code: 1}, 0)
	if session.PauseRetiredCapture() || session.PhysicalPressCount() != 1 || session.capturePaused {
		t.Fatal("retirement changed a held physical ledger")
	}
	routePhysicalAction(session, nil, control.InputAction{Kind: control.InputKeyUp, Code: 1}, 0)
	session.Retire()
	if session.PauseRetiredCapture() {
		t.Fatal("retirement revived a terminal sender")
	}
}
