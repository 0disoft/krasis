//go:build windows

package wininput

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"golang.org/x/sys/windows"
	"runtime"
	"testing"
	"time"
)

func TestSourceDispatcherPriorityOwnsActualDesktopThreadAndRestores(t *testing.T) {
	before := sourceCommandOwnerDesktop
	t.Cleanup(func() { sourceCommandOwnerDesktop = before })
	failure := errors.New("original dispatcher result")
	var ownerID uint32
	sourceCommandOwnerDesktop = func(_ context.Context, authorize func() error, callback func(*inputdesktop.Scope) error) error {
		// Model the real inputdesktop owner rather than its calling goroutine.
		result := make(chan error, 1)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ownerID = windows.GetCurrentThreadId()
			if err := authorize(); err != nil {
				result <- err
				return
			}
			result <- callback(&inputdesktop.Scope{})
		}()
		return <-result
	}
	hook, _ := NewProtectedSourceHook(func() error { return nil })
	var thread windows.Handle
	err := hook.WithSourceCommandDesktop(context.Background(), func(ctx context.Context) error {
		if windows.GetCurrentThreadId() != ownerID || currentSourceThreadPriority() != sourceInputPriorityAboveNormal {
			t.Error("dispatcher stayed on caller or normal-priority thread")
		}
		var err error
		thread, err = windows.OpenThread(windows.THREAD_QUERY_INFORMATION, false, ownerID)
		if err != nil {
			t.Error(err)
		}
		if scope := ctx.Value(sourceCommandDesktopKey{}); scope == nil {
			t.Error("lost existing desktop scope")
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal("priority scope changed command result", err)
	}
	if thread == 0 {
		t.Fatal("missing desktop thread handle")
	}
	defer windows.CloseHandle(thread)
	deadline := time.Now().Add(time.Second)
	for {
		p, _, _ := sourceGetThreadPriority.Call(uintptr(thread))
		if int32(p) == sourceInputPriorityNormal {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("priority leaked after dispatcher return", int32(p))
		}
		runtime.Gosched()
	}
}
