//go:build windows

package winlifecycle

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/0disoft/krasis/internal/agentlifecycle"
)

const notifyForThisSession = 0

var lifecycleClassSequence atomic.Uint64

var (
	user32                               = syscall.NewLazyDLL("user32.dll")
	kernel32                             = syscall.NewLazyDLL("kernel32.dll")
	wtsapi32                             = syscall.NewLazyDLL("wtsapi32.dll")
	procRegisterClassW                   = user32.NewProc("RegisterClassW")
	procUnregisterClassW                 = user32.NewProc("UnregisterClassW")
	procCreateWindowExW                  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW                   = user32.NewProc("DefWindowProcW")
	procDestroyWindow                    = user32.NewProc("DestroyWindow")
	procGetMessageW                      = user32.NewProc("GetMessageW")
	procTranslateMessage                 = user32.NewProc("TranslateMessage")
	procDispatchMessageW                 = user32.NewProc("DispatchMessageW")
	procPostMessageW                     = user32.NewProc("PostMessageW")
	procPostThreadMessageW               = user32.NewProc("PostThreadMessageW")
	procPostQuitMessage                  = user32.NewProc("PostQuitMessage")
	procGetModuleHandleW                 = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID               = kernel32.NewProc("GetCurrentThreadId")
	procWTSRegisterSessionNotification   = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnRegisterSessionNotification = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
)

type Monitor struct {
	events chan agentlifecycle.Event
	done   chan struct{}
	ready  chan error

	window          atomic.Uintptr
	thread          atomic.Uint32
	close           sync.Once
	mu              sync.Mutex
	runErr          error
	windowDestroyed bool // accessed only on the native owner thread
	tearingDown     bool // suppress a new WM_QUIT after the message loop has exited
}

type windowClass struct {
	style       uint32
	wndProc     uintptr
	classExtra  int32
	windowExtra int32
	instance    uintptr
	icon        uintptr
	cursor      uintptr
	background  uintptr
	menuName    *uint16
	className   *uint16
}

type point struct {
	x int32
	y int32
}

type windowMessage struct {
	window  uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   point
	private uint32
}

func StartMonitor() (agentlifecycle.Monitor, error) {
	monitor := &Monitor{
		events: make(chan agentlifecycle.Event, 1),
		done:   make(chan struct{}),
		ready:  make(chan error, 1),
	}
	go monitor.run()
	if err := <-monitor.ready; err != nil {
		<-monitor.done
		return nil, err
	}
	return monitor, nil
}

func (monitor *Monitor) Events() <-chan agentlifecycle.Event { return monitor.events }
func (monitor *Monitor) Done() <-chan struct{}               { return monitor.done }

func (monitor *Monitor) Close() error {
	if monitor == nil {
		return nil
	}
	monitor.close.Do(func() {
		select {
		case <-monitor.done:
			return
		default:
		}
		window := monitor.window.Load()
		if window != 0 {
			if result, _, callErr := procPostMessageW.Call(window, wmLifecycleStop, 0, 0); result == 0 {
				monitor.setRunError(win32Error("PostMessageW(lifecycle stop)", callErr))
				thread := monitor.thread.Load()
				if posted, _, postErr := procPostThreadMessageW.Call(uintptr(thread), wmQuit, 0, 0); posted == 0 {
					monitor.setRunError(win32Error("PostThreadMessageW(WM_QUIT)", postErr))
				}
			}
		}
		<-monitor.done
	})
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	return monitor.runErr
}

func (monitor *Monitor) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(monitor.done)
	defer close(monitor.events)
	thread, _, _ := procGetCurrentThreadID.Call()
	monitor.thread.Store(uint32(thread))
	defer monitor.thread.Store(0)

	className, err := syscall.UTF16PtrFromString(fmt.Sprintf("KrasisLifecycle-%d-%d", os.Getpid(), lifecycleClassSequence.Add(1)))
	if err != nil {
		monitor.ready <- err
		return
	}
	module, _, callErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		monitor.ready <- win32Error("GetModuleHandleW", callErr)
		return
	}
	class := windowClass{wndProc: lifecycleWindowCallback, instance: module, className: className}
	atom, _, callErr := procRegisterClassW.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		monitor.ready <- win32Error("RegisterClassW", callErr)
		return
	}
	registered := true
	defer func() {
		if registered {
			if result, _, unregisterErr := procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), module); result == 0 {
				monitor.setRunError(win32Error("UnregisterClassW", unregisterErr))
			}
		}
	}()
	window, _, callErr := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0,
		0, 0, 0, 0, 0, 0, module, 0,
	)
	if window == 0 {
		monitor.ready <- win32Error("CreateWindowExW", callErr)
		return
	}
	monitor.window.Store(window)
	lifecycleWindowOwners.Store(window, monitor)
	defer lifecycleWindowOwners.CompareAndDelete(window, monitor)
	defer monitor.window.Store(0)
	defer func() {
		monitor.tearingDown = true
		if !monitor.windowDestroyed {
			if result, _, destroyErr := procDestroyWindow.Call(window); result == 0 {
				monitor.setRunError(win32Error("DestroyWindow", destroyErr))
			}
		}
	}()
	if result, _, callErr := procWTSRegisterSessionNotification.Call(window, notifyForThisSession); result == 0 {
		monitor.ready <- win32Error("WTSRegisterSessionNotification", callErr)
		return
	}
	defer func() {
		if result, _, unregisterErr := procWTSUnRegisterSessionNotification.Call(window); result == 0 {
			monitor.setRunError(win32Error("WTSUnRegisterSessionNotification", unregisterErr))
		}
	}()
	stopDesktopWatch, err := monitor.watchDesktopSwitch()
	if err != nil {
		monitor.ready <- err
		return
	}
	defer stopDesktopWatch()
	monitor.ready <- nil

	var message windowMessage
	for {
		result, _, getErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch int32(result) {
		case -1:
			monitor.setRunError(win32Error("GetMessageW", getErr))
			return
		case 0:
			return
		default:
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
		}
	}
}

func (monitor *Monitor) publish(event agentlifecycle.Event) {
	select {
	case monitor.events <- event:
	default:
	}
}

func (monitor *Monitor) setRunError(err error) {
	if err == nil {
		return
	}
	monitor.mu.Lock()
	monitor.runErr = errors.Join(monitor.runErr, err)
	monitor.mu.Unlock()
}

func win32Error(operation string, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return errors.New(operation + " failed")
	}
	return fmt.Errorf("%s failed: %w", operation, err)
}
