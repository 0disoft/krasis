//go:build windows

// Package winmessage keeps idle USER message queues out of unbounded syscalls.
package winmessage

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// MaximumWaitMillis bounds a single native wait, not message delivery. Posted
// input wakes it immediately. Returning to Go also lets GC finish taking a P
// missed during syscall entry (golang/go#81759), even on a completely idle queue.
const MaximumWaitMillis = 50

const quit = 0x0012

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	peek   = user32.NewProc("PeekMessageW")
	wait   = user32.NewProc("MsgWaitForMultipleObjectsEx")
)

type Point struct{ X, Y int32 }

// Message has the native MSG layout, including the reserved DWORD.
type Message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          Point
	Private        uint32
}

// Get preserves GetMessageW's message/WM_QUIT/error result without leaving the
// calling goroutine in one indefinite syscall. Call on the queue's locked owner.
func Get(message *Message) (int32, error) {
	for {
		available, _, _ := peek.Call(uintptr(unsafe.Pointer(message)), 0, 0, 0, 1)
		runtime.KeepAlive(message)
		if available != 0 {
			if message.Message == quit {
				return 0, nil
			}
			return 1, nil
		}
		if _, err := waitForMessage(); err != nil {
			return -1, err
		}
	}
}

func waitForMessage() (bool, error) {
	// MWMO_INPUTAVAILABLE is required for messages PeekMessage already saw.
	result, _, err := wait.Call(0, 0, MaximumWaitMillis, 0x04ff, 0x0004)
	switch uint32(result) {
	case 0:
		return true, nil // WAIT_OBJECT_0: message queue signalled.
	case 258:
		return false, nil // WAIT_TIMEOUT: return to Go before waiting again.
	default:
		if err == syscall.Errno(0) {
			err = errors.New("unexpected message wait result")
		}
		return false, fmt.Errorf("MsgWaitForMultipleObjectsEx (%d): %w", result, err)
	}
}
