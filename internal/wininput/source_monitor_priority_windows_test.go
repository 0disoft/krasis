//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Cancel at hook-owner admission, after monitor setup but before native hooks.
// This exercises the real monitor's early-return cleanup without input capture.
type sourceMonitorCancelContext struct {
	context.Context
	calls    int
	priority int32
	thread   windows.Handle
	openErr  error
}

func (c *sourceMonitorCancelContext) Err() error {
	c.calls++
	if c.calls == 1 {
		return nil
	}
	c.priority = currentSourceThreadPriority()
	c.thread, c.openErr = windows.OpenThread(windows.THREAD_QUERY_INFORMATION, false, windows.GetCurrentThreadId())
	return context.Canceled
}

func TestSourceMonitorPriorityRestoredAfterCancelledAdmission(t *testing.T) {
	ctx := &sourceMonitorCancelContext{Context: context.Background()}
	session, _ := NewSourceSession(1, &recordingSourceSink{})
	result := make(chan error, 1)
	go func() { result <- NewSourceHook().Run(ctx, session, nil) }()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("monitor lost cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("monitor cancellation did not complete")
	}
	if ctx.openErr != nil {
		t.Fatal(ctx.openErr)
	}
	defer windows.CloseHandle(ctx.thread)
	if ctx.calls != 2 || ctx.priority != sourceInputPriorityAboveNormal {
		t.Fatal("foreground monitor retained normal priority", ctx.calls, ctx.priority)
	}
	after, _, _ := sourceGetThreadPriority.Call(uintptr(ctx.thread))
	if int32(after) != sourceInputPriorityNormal {
		t.Fatal("monitor priority leaked to subsequent Go work", int32(after))
	}
}
