//go:build windows

package winlifecycle

import (
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestMonitorStartsAndClosesHiddenWindow(t *testing.T) {
	monitor, err := StartMonitor()
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- monitor.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Windows lifecycle monitor did not close")
	}
}

func TestLifecycleNativeWindowsRetireOwnersAcrossRepeatedStarts(t *testing.T) {
	callback := lifecycleWindowCallback
	isWindow := user32.NewProc("IsWindow")
	getGUI := user32.NewProc("GetGuiResources")
	currentProcess := kernel32.NewProc("GetCurrentProcess")
	getHandles := kernel32.NewProc("GetProcessHandleCount")
	process, _, _ := currentProcess.Call()
	openThread := kernel32.NewProc("OpenThread")
	waitThread := kernel32.NewProc("WaitForSingleObject")
	closeHandle := kernel32.NewProc("CloseHandle")
	joinNativeThread := func(monitor *Monitor) {
		if monitor.thread.Load() == startupThreadID {
			t.Fatal("lifecycle owner used the non-terminating Go startup thread")
		}
		handle, _, err := openThread.Call(0x00100000, 0, uintptr(monitor.thread.Load())) // SYNCHRONIZE only.
		if handle == 0 {
			t.Fatalf("OpenThread: %v", err)
		}
		defer closeHandle.Call(handle)
		if err := monitor.Close(); err != nil {
			t.Fatal(err)
		}
		if result, _, err := waitThread.Call(handle, 2000); result != 0 {
			t.Fatalf("native owner thread did not exit: result=%d error=%v", result, err)
		}
	}
	resources := func() (uintptr, uint32) {
		gui, _, _ := getGUI.Call(process, 1)
		var handles uint32
		ok, _, err := getHandles.Call(process, uintptr(unsafe.Pointer(&handles)))
		if ok == 0 {
			t.Fatalf("GetProcessHandleCount: %v", err)
		}
		return gui, handles
	}
	// Warm up Win32/Go thread caches before measuring a bounded 100-owner run.
	warm, err := StartMonitor()
	if err != nil {
		t.Fatal(err)
	}
	joinNativeThread(warm.(*Monitor))
	beforeGUI, beforeHandles := resources()
	for i := 0; i < 100; i++ {
		monitorInterface, err := StartMonitor()
		if err != nil {
			t.Fatal(err)
		}
		monitor := monitorInterface.(*Monitor)
		window := monitor.window.Load()
		posted, _, err := procPostMessageW.Call(window, wmDisplayChange, 0, 0)
		if posted == 0 {
			monitor.Close()
			t.Fatalf("post display change: %v", err)
		}
		select {
		case event := <-monitor.Events():
			if event != agentlifecycle.EventDisplayChange {
				monitor.Close()
				t.Fatalf("wrong owner event: %v", event)
			}
		case <-time.After(time.Second):
			monitor.Close()
			t.Fatal("window event not routed")
		}
		joinNativeThread(monitor)
		if exists, _, _ := isWindow.Call(window); exists != 0 {
			t.Fatal("window survived Close")
		}
		if _, ok := lifecycleWindowOwners.Load(window); ok {
			t.Fatal("retired callback retains monitor")
		}
		if monitor.window.Load() != 0 || monitor.thread.Load() != 0 {
			t.Fatal("native owner did not finish")
		}
	}
	afterGUI, afterHandles := resources()
	// Allow bounded runtime handle cache noise; a per-session leak exceeds this.
	if afterGUI > beforeGUI+2 || afterHandles > beforeHandles+16 {
		t.Fatalf("native resources grew: GUI %d->%d handles %d->%d", beforeGUI, afterGUI, beforeHandles, afterHandles)
	}
	if callback != lifecycleWindowCallback {
		t.Fatal("callback trampoline changed")
	}
	// An unknown old window must use DefWindowProc, not reach a retired monitor.
	syscall.SyscallN(lifecycleWindowCallback, 0, wmDisplayChange, 0, 0)
}
