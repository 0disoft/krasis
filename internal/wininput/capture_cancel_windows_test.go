//go:build windows

package wininput

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestCaptureLoopFailureDoesNotMaskWakeFailureWithCancellation(t *testing.T) {
	wake := errors.New("wake event failed")
	result := captureLoopFailure(errors.Join(context.Canceled, sourceStage("message-loop", wake)))
	if !errors.Is(result, wake) || errors.Is(result, context.Canceled) {
		t.Fatal("wake failure hidden by cancellation", result)
	}
	if captureLoopFailure(context.Canceled) != nil {
		t.Fatal("ordinary cancellation became a wake failure")
	}
}

func TestCaptureMessageLoopCancellationAfterReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		thread, _, _ := procGetCurrentThreadID.Call()
		var bootstrap message
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrap)), 0, 0, 0, pmNoRemove)
		if ok, _, err := procPostThreadMessageW.Call(thread, wmCaptureReady, 0, 0); ok == 0 {
			done <- err
			return
		}
		done <- runMessageLoop(ctx, func() { close(ready) })
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("loop stopped before readiness: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not become ready")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not stop after cancellation")
	}
}
