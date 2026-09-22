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
	if err = warm.Close(); err != nil {
		t.Fatal(err)
	}
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
		if err = monitor.Close(); err != nil {
			t.Fatal(err)
		}
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
