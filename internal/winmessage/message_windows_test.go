//go:build windows

package winmessage

import (
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// These tests own only private thread queues: no input injection, visible
// windows, desktop changes or messages to existing application threads.
func privateQueue(t *testing.T, run func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var bootstrap Message
		peek.Call(uintptr(unsafe.Pointer(&bootstrap)), 0, 0, 0, 0)
		done <- run()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("private message queue did not return")
	}
}

func TestIdleNativeWaitReturnsWithoutGoTimerOrMessage(t *testing.T) {
	privateQueue(t, func() error {
		ready, err := waitForMessage()
		if err == nil && ready {
			t.Error("idle queue reported a message")
		}
		return err
	})
}

func TestGetPreservesExaminedMessagesAndQuit(t *testing.T) {
	privateQueue(t, func() error {
		thread, _, _ := syscallThreadID.Call()
		post := user32.NewProc("PostThreadMessageW")
		for _, message := range []uintptr{0x8001, 0x8002, quit} {
			if result, _, err := post.Call(thread, message, 37, 91); result == 0 {
				return err
			}
		}
		var message Message
		peek.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 0)
		// Seeing a message without consuming it must not make the wait sleep.
		ready, err := waitForMessage()
		if err != nil {
			return err
		}
		if !ready {
			t.Error("already examined message did not wake native wait")
		}
		for i, want := range []uint32{0x8001, 0x8002, quit} {
			result, err := Get(&message)
			if err != nil {
				return err
			}
			if message.Message != want || message.WParam != 37 || message.LParam != 91 {
				t.Errorf("message order/payload changed: %+v", message)
			}
			if (result == 0) != (i == 2) {
				t.Errorf("wrong quit result %d", result)
			}
		}
		return nil
	})
}

var syscallThreadID = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentThreadId")
