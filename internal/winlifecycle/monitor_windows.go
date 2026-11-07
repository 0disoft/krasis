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
	"github.com/0disoft/krasis/internal/winmessage"
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
	procTranslateMessage                 = user32.NewProc("TranslateMessage")
	procDispatchMessageW                 = user32.NewProc("DispatchMessageW")
	procPostMessageW                     = user32.NewProc("PostMessageW")
	procPostThreadMessageW               = user32.NewProc("PostThreadMessageW")
	procPostQuitMessage                  = user32.NewProc("PostQuitMessage")
	procPeekLifecycleMessage             = user32.NewProc("PeekMessageW")
	procWaitLifecycleMessage             = user32.NewProc("MsgWaitForMultipleObjectsEx")
	procGetModuleHandleW                 = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID               = kernel32.NewProc("GetCurrentThreadId")
	procWTSRegisterSessionNotification   = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procWTSUnRegisterSessionNotification = wtsapi32.NewProc("WTSUnRegisterSessionNotification")
	startupThreadID                      = lifecycleThreadID()
)

type Monitor struct {
	events chan agentlifecycle.Event
	done   chan struct{}
	ready  chan error
	stop   chan struct{}

	window          atomic.Uintptr
	thread          atomic.Uint32
	close           sync.Once
	mu              sync.Mutex
	runErr          error
	windowDestroyed bool                        // accessed only on the native owner thread
	tearingDown     bool                        // suppress a new WM_QUIT after the message loop has exited
	postStop        func(uintptr, uint32) error // Immutable optional test seam.
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

type windowMessage = winmessage.Message

func StartMonitor() (agentlifecycle.Monitor, error) {
	monitor := &Monitor{
		events: make(chan agentlifecycle.Event, 1),
		done:   make(chan struct{}),
		ready:  make(chan error, 1),
		stop:   make(chan struct{}),
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
			post := postLifecycleStop
			if monitor.postStop != nil {
				post = monitor.postStop
			}
			monitor.setRunError(post(window, monitor.thread.Load()))
		}
		// Attempt the wake before releasing the independent stop signal: the
		// owner must not destroy its window underneath our normal PostMessage.
		close(monitor.stop)
		<-monitor.done
	})
	monitor.mu.Lock()
	defer monitor.mu.Unlock()
	return monitor.runErr
}

func (monitor *Monitor) run() {
	runtime.LockOSThread()
	thread := lifecycleThreadID()
	if thread == startupThreadID {
		// Go parks m0 instead of terminating it when a locked goroutine exits.
		// Keep m0 occupied until another thread owns the whole monitor lifetime.
		defer runtime.UnlockOSThread()
		finished := make(chan struct{})
		go func() { monitor.run(); close(finished) }()
		<-finished
		return
	}
	defer close(monitor.done)
	// Like the input owners, this goroutine deliberately exits while locked.
	// Go then terminates the dedicated OS thread instead of caching its
	// thread-local USER/WTS resources for an unrelated future goroutine.
	defer close(monitor.events)
	monitor.thread.Store(thread)
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

	monitor.setRunError(monitor.messageLoop())
}

func postLifecycleStop(window uintptr, thread uint32) error {
	if result, _, callErr := procPostMessageW.Call(window, wmLifecycleStop, 0, 0); result == 0 {
		failure := win32Error("PostMessageW(lifecycle stop)", callErr)
		if posted, _, postErr := procPostThreadMessageW.Call(uintptr(thread), wmQuit, 0, 0); posted == 0 {
			failure = errors.Join(failure, win32Error("PostThreadMessageW(WM_QUIT)", postErr))
		}
		return failure
	}
	return nil
}

// Stop does not depend on either posted message reaching the queue. Check it
// between messages and bounded waits, then clean up on this native owner.
func (monitor *Monitor) messageLoop() error {
	var message windowMessage
	for {
		select {
		case <-monitor.stop:
			return nil
		default:
		}
		available, _, _ := procPeekLifecycleMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 1)
		if available != 0 {
			if message.Message == wmQuit {
				return nil
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
			continue
		}
		state, _, callErr := procWaitLifecycleMessage.Call(0, 0, winmessage.MaximumWaitMillis, 0x04ff, 0x0004)
		if state != 0 && state != 258 { // Message available or WAIT_TIMEOUT.
			return win32Error("MsgWaitForMultipleObjectsEx(lifecycle)", callErr)
		}
	}
}

func lifecycleThreadID() uint32 { thread, _, _ := procGetCurrentThreadID.Call(); return uint32(thread) }

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
