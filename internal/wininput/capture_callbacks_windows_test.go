//go:build windows

package wininput

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestCaptureCallbackTrampolinesRouteRepeatedSessions(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	thread, _, _ := procGetCurrentThreadID.Call()
	keyboard, mouse := sourceKeyboardCallback, sourceMouseCallback
	// Windows owns hook payload memory. Use native allocations here as well;
	// round-tripping a Go pointer through the native ABI breaks checkptr tracking.
	key, err := windows.LocalAlloc(windows.LPTR, uint32(unsafe.Sizeof(keyboardHookData{})))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.LocalFree(windows.Handle(key))
	pointer, err := windows.LocalAlloc(windows.LPTR, uint32(unsafe.Sizeof(mouseHookData{})))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.LocalFree(windows.Handle(pointer))
	*(*keyboardHookData)(unsafe.Pointer(key)) = keyboardHookData{Flags: llkhfInjected}
	*(*mouseHookData)(unsafe.Pointer(pointer)) = mouseHookData{Flags: llmhfInjected, ExtraInfo: selfTestExtraInfo}
	var last *captureCounters
	for range 10000 {
		counters := &captureCounters{}
		release, err := registerCaptureHookOwner(thread, counters)
		if err != nil {
			t.Fatal(err)
		}
		// Call the actual native trampolines on their owning thread. No global
		// input hook is installed and no keyboard or mouse input is generated.
		syscall.SyscallN(keyboard, hcAction, wmSysKeyDown, key)
		syscall.SyscallN(mouse, hcAction, wmMouseMove, pointer)
		release()
		if _, retained := sourceHookOwners.Load(thread); retained {
			t.Fatal("session counters remain reachable from callback registry")
		}
		snapshot := counters.snapshot(0)
		if snapshot.Keyboard.InjectedDown != 1 || snapshot.Keyboard.PhysicalDown != 0 || snapshot.Mouse.SelfTestInjectedMove != 1 || snapshot.Mouse.InjectedMove != 1 || snapshot.Mouse.PhysicalMove != 0 {
			t.Fatal("native callbacks reached another session or changed counters", snapshot)
		}
		last = counters
	}
	if sourceKeyboardCallback != keyboard || sourceMouseCallback != mouse {
		t.Fatal("capture replaced a process-lifetime trampoline")
	}
	// A callback after removal must pass through, not reach an old session.
	syscall.SyscallN(keyboard, hcAction, wmSysKeyDown, key)
	syscall.SyscallN(mouse, hcAction, wmMouseMove, pointer)
	if last.keyboardInjectedDown.Load() != 1 || last.mouseInjectedMove.Load() != 1 {
		t.Fatal("callback after removal reached the previous session")
	}
}

func TestCaptureOwnerCollisionAndStaleCleanupCannotReplaceAnotherSession(t *testing.T) {
	thread := ^uintptr(0) - 1 // Not an OS thread; never install a native hook.
	first, second := &captureCounters{}, &captureCounters{}
	releaseFirst, err := registerCaptureHookOwner(thread, first)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFirst()
	owner, _ := sourceHookOwners.Load(thread)
	if release, err := registerCaptureHookOwner(thread, second); err == nil || release != nil {
		t.Fatal("duplicate registration replaced a live callback owner")
	}
	if current, _ := sourceHookOwners.Load(thread); current != owner {
		t.Fatal("failed registration changed original owner")
	}
	releaseFirst()
	releaseSecond, err := registerCaptureHookOwner(thread, second)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSecond()
	owner, _ = sourceHookOwners.Load(thread)
	releaseFirst() // Old cleanup must not remove an owner after thread-ID reuse.
	if current, _ := sourceHookOwners.Load(thread); current != owner {
		t.Fatal("stale cleanup removed the next owner")
	}
	releaseSecond()
	if _, retained := sourceHookOwners.Load(thread); retained {
		t.Fatal("capture owner was retained")
	}
}

func TestCaptureOwnerRejectsMissingThreadAndCounters(t *testing.T) {
	for _, test := range []struct {
		thread   uintptr
		counters *captureCounters
	}{{0, &captureCounters{}}, {1, nil}} {
		if release, err := registerCaptureHookOwner(test.thread, test.counters); err == nil || release != nil {
			t.Fatal("invalid owner was registered")
		}
	}
}
