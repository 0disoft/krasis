//go:build windows

package wininput

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLoginSourceHookRejectsOrdinaryProcessBeforeCapture(t *testing.T) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if user.User.Sid.IsWellKnown(windows.WinLocalSystemSid) {
		t.Skip("ordinary caller required")
	}
	hook, err := NewLoginSourceHook(func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ready := false
	// Rejection must precede all capture setup, even with claimed authorization.
	if err := hook.onDesktop(context.Background(), func(context.Context) error { ready = true; return nil }); err == nil || ready {
		t.Fatalf("ordinary process captured login input: err=%v ready=%v", err, ready)
	}
	if _, err := NewLoginSourceHook(nil); err == nil {
		t.Fatal("missing authorization admitted")
	}
}

func TestLoginCaptureDoesNotStartUserShellPresentation(t *testing.T) {
	old := sourceCaptureMarkFullscreen
	defer func() { sourceCaptureMarkFullscreen = old }()
	calls := 0
	sourceCaptureMarkFullscreen = func(uintptr, bool) bool { calls++; return true }
	current := &sourceWheelCaptureRun{loginContext: context.Background(), fullscreenAttempted: true}
	current.inheritFullscreenPresentation()
	current.releaseFullscreenPresentation()
	if calls != 0 {
		t.Fatal("login capture entered user Shell broker")
	}
}
