//go:build windows

package winlifecycle

import (
	"errors"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestMonitorCloseJoinsOwnerWhenBothStopMessagesFail(t *testing.T) {
	owner, err := StartMonitor()
	if err != nil {
		t.Fatal(err)
	}
	monitor := owner.(*Monitor)
	window := monitor.window.Load()
	thread, _, openErr := kernel32.NewProc("OpenThread").Call(0x00100000, 0, uintptr(monitor.thread.Load()))
	if thread == 0 {
		_ = monitor.Close()
		t.Fatal("OpenThread", openErr)
	}
	defer kernel32.NewProc("CloseHandle").Call(thread)
	windowPost, threadPost := errors.New("window post failed"), errors.New("thread post failed")
	monitor.postStop = func(uintptr, uint32) error { return errors.Join(windowPost, threadPost) }
	done := make(chan error, 1)
	go func() { done <- monitor.Close() }()
	select {
	case err := <-done:
		if !errors.Is(err, windowPost) || !errors.Is(err, threadPost) {
			t.Fatal("stop delivery errors lost", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close waited for undeliverable stop messages")
	}
	if state, _, err := kernel32.NewProc("WaitForSingleObject").Call(thread, 2000); state != 0 {
		t.Fatal("native thread survived failed stop delivery", state, err)
	}
	if exists, _, _ := user32.NewProc("IsWindow").Call(window); exists != 0 {
		t.Fatal("window survived failed stop delivery")
	}
	if _, exists := lifecycleWindowOwners.Load(window); exists || monitor.thread.Load() != 0 {
		t.Fatal("native owner survived Close")
	}
	desktopSwitchOwners.Range(func(_, value any) bool {
		if value == monitor {
			t.Error("desktop hook retained retired monitor")
		}
		return true
	})
	if err := monitor.Close(); !errors.Is(err, windowPost) || !errors.Is(err, threadPost) {
		t.Fatal("repeated Close lost original errors", err)
	}
}

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
	getClassName := user32.NewProc("GetClassNameW")
	getClassInfo := user32.NewProc("GetClassInfoW")
	module, _, _ := procGetModuleHandleW.Call(0)
	if module == 0 {
		t.Fatal("test module unavailable")
	}
	joinNativeThread := func(monitor *Monitor) {
		window := monitor.window.Load()
		var name [256]uint16
		if size, _, err := getClassName.Call(window, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name))); size == 0 {
			t.Fatalf("GetClassNameW: %v", err)
		}
		if monitor.thread.Load() == startupThreadID {
			t.Fatal("lifecycle owner used the non-terminating Go startup thread")
		}
		handle, _, err := openThread.Call(0x00100000, 0, uintptr(monitor.thread.Load())) // SYNCHRONIZE only.
		if handle == 0 {
			t.Fatalf("OpenThread: %v", err)
		}
		defer func() {
			if ok, _, err := closeHandle.Call(handle); ok == 0 {
				t.Errorf("CloseHandle(test thread pin): %v", err)
			}
		}()
		if err := monitor.Close(); err != nil {
			t.Fatal(err)
		}
		if result, _, err := waitThread.Call(handle, 2000); result != 0 {
			t.Fatalf("native owner thread did not exit: result=%d error=%v", result, err)
		}
		var class windowClass
		exists, _, classErr := getClassInfo.Call(module, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&class)))
		if exists != 0 {
			t.Fatal("retired owner retained its registered class")
		}
		if classErr != syscall.Errno(1411) { // ERROR_CLASS_DOES_NOT_EXIST.
			t.Fatal("class retirement could not be confirmed", classErr)
		}
		desktopSwitchOwners.Range(func(_, owner any) bool {
			if owner == monitor {
				t.Error("retired owner retained its desktop event hook")
			}
			return true
		})
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
	// Process-wide totals include Go/USER initialization and concurrent activity.
	// They cannot attribute a leak to this owner. Exact thread/window/class/hook
	// retirement above remains mandatory; retain totals as diagnostic evidence.
	t.Logf("process resource totals (not owner counts): GUI %d->%d handles %d->%d", beforeGUI, afterGUI, beforeHandles, afterHandles)
	if callback != lifecycleWindowCallback {
		t.Fatal("callback trampoline changed")
	}
	// An unknown old window must use DefWindowProc, not reach a retired monitor.
	syscall.SyscallN(lifecycleWindowCallback, 0, wmDisplayChange, 0, 0)
}
