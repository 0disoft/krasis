//go:build windows

package residentagent

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/wininput"
)

type protectedTestHook func(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error

func (f protectedTestHook) RunWithBoundary(c context.Context, s *wininput.SourceSession, b wininput.BoundaryObserver, r func()) error {
	return f(c, s, b, r)
}

func TestProtectedReceiverIsReadyWithoutReadingSecureCursor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, events := 0, 0
	h := &protectedSource{blocked: func() bool { return true }, event: func(code string) {
		if code != "installed_source_receive_only" {
			t.Fatal(code)
		}
		events++
	}, native: protectedTestHook(func(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error {
		t.Fatal("native capture called on secure desktop")
		return nil
	})}
	err := h.RunWithBoundary(ctx, &wininput.SourceSession{}, nil, func() { ready++; cancel() })
	if !errors.Is(err, context.Canceled) || ready != 1 || events != 1 {
		t.Fatalf("standby: %v %d %d", err, ready, events)
	}
	if h.PrepareReturnCursor() != nil {
		t.Fatal("secure desktop cursor queried")
	}
}

func TestProtectedReceiverDoesNotHideOtherNativeFailures(t *testing.T) {
	failure := errors.New("native cleanup failed")
	blocked := false
	h := &protectedSource{blocked: func() bool { return blocked }, event: func(string) { t.Fatal("unrelated failure hidden") }, native: protectedTestHook(func(context.Context, *wininput.SourceSession, wininput.BoundaryObserver, func()) error {
		blocked = true
		return failure
	})}
	if err := h.RunWithBoundary(context.Background(), &wininput.SourceSession{}, nil, func() { t.Fatal("false readiness") }); err != failure {
		t.Fatal(err)
	}
}

func TestProtectedSourceCannotActivateLocalCaptureOnSecureDesktop(t *testing.T) {
	c := &protectedSourceCapture{blocked: func() bool { return true }}
	if !errors.Is(c.Activate(), wininput.ErrProtectedInput) || !errors.Is(c.ActivateProfile(context.Background()), wininput.ErrProtectedInput) {
		t.Fatal("secure local capture allowed")
	}
}

type protectedCursorTest struct{ prepared bool }

func (c *protectedCursorTest) PrepareReturnCursor() func() {
	c.prepared = true
	return func() {}
}
func (*protectedCursorTest) RestoreReturnCursor() {}

func TestLockScreenUsesProtectedCursorWithoutBlockingNativeCapture(t *testing.T) {
	cursor := &protectedCursorTest{}
	h := &protectedSource{SourceHook: wininput.NewSourceHook(), prepared: true,
		blocked: func() bool { return false }, cursorProtected: func() bool { return true },
		protectedCursor: cursor}
	if h.PrepareReturnCursor() == nil || !cursor.prepared || !h.captureReady() || h.blocked() {
		t.Fatal("lock screen used medium cursor or changed capture readiness")
	}
	cursor.prepared = false
	h.cursorProtected = func() bool { return false }
	h.PrepareReturnCursor()
	if cursor.prepared {
		t.Fatal("ordinary desktop launched a privileged cursor owner")
	}
}
