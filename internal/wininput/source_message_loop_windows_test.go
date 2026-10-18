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

// Exercise real Win32 queues/events without installing hooks, capturing input,
// showing a window, changing desktops or posting to any application thread.
func TestSourceMessageLoopCancellationWithoutQuitMessage(t *testing.T) {
	for _, busy := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle", true: "busy"}[busy], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready := make(chan struct{})
			done := make(chan error, 1)
			go func() {
				runtime.LockOSThread() // Retire this private queue on return.
				var bootstrap message
				procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrap)), 0, 0, 0, pmNoRemove)
				thread, _, _ := procGetCurrentThreadID.Call()
				if err := postSourceReady(thread); err != nil {
					done <- err
					return
				}
				messages := 0
				done <- sourceMessageLoop(ctx, func(msg message) error {
					if msg.Message != wmSourceReady {
						return nil
					}
					messages++
					if messages == 1 {
						close(ready)
					}
					if busy {
						if messages == 32 {
							cancel()
						}
						return postSourceReady(thread) // Queue stays nonempty at cancellation.
					}
					return nil
				})
			}()
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("private message queue did not start")
			}
			if !busy {
				time.Sleep(20 * time.Millisecond) // Let the owner enter its native wait.
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation required a WM_QUIT message")
			}
		})
	}
}
