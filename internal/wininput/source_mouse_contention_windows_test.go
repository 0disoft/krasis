//go:build windows

package wininput

import (
	"testing"
	"time"
	"unsafe"
)

func TestPhysicalMouseSuppressionDoesNotWaitForSessionStateMutex(t *testing.T) {
	s := newInactiveSourceSession(t)
	s.active.Store(true)
	pointer := newSourcePointerState(point{})
	callbacks := newSourceLowLevelCallbacks(s, nil, nil, nil, pointer, nil, nil, func() error { return nil }, func(int, uintptr, uintptr) uintptr { return 42 })
	s.stateMu.Lock()
	locked := true
	defer func() {
		if locked {
			s.stateMu.Unlock()
		}
	}()
	done := make(chan uintptr, 1)
	go func() {
		data := mouseHookData{Point: point{X: 90}}
		done <- callbacks.mouse(hcAction, wmMouseMove, uintptr(unsafe.Pointer(&data)))
	}()
	select {
	case result := <-done:
		if result != 1 {
			t.Fatal("active physical mouse was not suppressed", result)
		}
	case <-time.After(time.Second):
		s.stateMu.Unlock()
		locked = false
		<-done
		t.Fatal("physical mouse callback waited for state transaction")
	}
	if x, known := pointer.pointerX(); !known || x != 90 {
		t.Fatal("pointer observation lost")
	}
	// Retirement must win even when the prior active bit remains published
	// until the ordinary worker obtains the state transaction lock.
	s.retireHookFailure(errHookCallbackStalled)
	data := mouseHookData{}
	if callbacks.mouse(hcAction, wmMouseMove, uintptr(unsafe.Pointer(&data))) != 42 || s.Active() {
		t.Fatal("retired source still suppressed input")
	}
}
