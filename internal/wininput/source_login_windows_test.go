//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
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
	protected, _ := NewProtectedSourceHook(func() error { return nil })
	if err := protected.onDesktop(context.Background(), func(context.Context) error { ready = true; return nil }); err == nil || ready {
		t.Fatal("ordinary process entered disposable protected capture", err)
	}
	if err := protected.ExecuteSourceDesktop(context.Background(), func() error { ready = true; return nil }); err == nil || ready {
		t.Fatal("ordinary process entered protected command desktop", err)
	}
}

func TestProtectedSourceCommandPreservesCleanupFailureAndCancellation(t *testing.T) {
	old := sourceCommandDesktop
	defer func() { sourceCommandDesktop = old }()
	var calls int
	failure := errors.Join(inputdesktop.ErrCleanup, errors.New("synthetic restore failure"))
	sourceCommandDesktop = func(ctx context.Context, authorize func() error, callback func(*inputdesktop.Scope) error) error {
		calls++
		if ctx.Err() != nil || authorize() != nil || callback == nil {
			t.Fatal("missing live scope or authentication")
		}
		return failure
	}
	hook, _ := NewProtectedSourceHook(func() error { return nil })
	if err := hook.ExecuteSourceDesktop(context.Background(), func() error { t.Fatal("callback escaped scope"); return nil }); !errors.Is(err, inputdesktop.ErrCleanup) {
		t.Fatal("restore failure was erased", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := hook.ExecuteSourceDesktop(ctx, func() error { return nil }); err != context.Canceled || calls != 1 {
		t.Fatal("cancelled command acquired another desktop", err, calls)
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
