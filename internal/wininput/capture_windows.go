//go:build windows

package wininput

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/lab"
)

const (
	whKeyboardLL      = 13
	whMouseLL         = 14
	hcAction          = 0
	wmQuit            = 0x0012
	wmCaptureReady    = 0x8001
	wmKeyDown         = 0x0100
	wmKeyUp           = 0x0101
	wmSysKeyDown      = 0x0104
	wmSysKeyUp        = 0x0105
	wmMouseMove       = 0x0200
	wmLButtonDown     = 0x0201
	wmLButtonUp       = 0x0202
	wmRButtonDown     = 0x0204
	wmRButtonUp       = 0x0205
	wmMButtonDown     = 0x0207
	wmMButtonUp       = 0x0208
	wmMouseWheel      = 0x020A
	wmXButtonDown     = 0x020B
	wmXButtonUp       = 0x020C
	wmMouseHWheel     = 0x020E
	llkhfInjected     = 0x00000010
	llmhfInjected     = 0x00000001
	pmNoRemove        = 0x0000
	selfTestExtraInfo = 0x4B525349
)

var (
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPeekMessageW        = user32.NewProc("PeekMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	procGetCurrentThreadID  = kernel32.NewProc("GetCurrentThreadId")
)

type point struct {
	X int32
	Y int32
}

type message struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

type keyboardHookData struct {
	VirtualKey uint32
	ScanCode   uint32
	Flags      uint32
	Time       uint32
	ExtraInfo  uintptr
}

type mouseHookData struct {
	Point     point
	MouseData uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type captureCounters struct {
	keyboardPhysicalDown      atomic.Uint64
	keyboardPhysicalUp        atomic.Uint64
	keyboardInjectedDown      atomic.Uint64
	keyboardInjectedUp        atomic.Uint64
	keyboardOther             atomic.Uint64
	mousePhysicalMove         atomic.Uint64
	mouseInjectedMove         atomic.Uint64
	mousePhysicalButtonDown   atomic.Uint64
	mousePhysicalButtonUp     atomic.Uint64
	mouseInjectedButtonDown   atomic.Uint64
	mouseInjectedButtonUp     atomic.Uint64
	mousePhysicalWheel        atomic.Uint64
	mouseInjectedWheel        atomic.Uint64
	mouseOther                atomic.Uint64
	mouseSelfTestInjectedMove atomic.Uint64
}

func (p *Platform) Capture(ctx context.Context, options lab.CaptureOptions, report func(lab.CaptureSnapshot)) error {
	if options.ReportInterval <= 0 {
		return fmt.Errorf("report interval must be positive")
	}
	if report == nil {
		return fmt.Errorf("capture reporter is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	releaseHookSession, err := acquireHookSession(ctx)
	if err != nil {
		return err
	}
	defer releaseHookSession()

	completed := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		// Returning while locked retires the OS thread. A later hook session must not
		// inherit this session's Win32 message queue or low-level hook state.
		completed <- captureOnDedicatedThread(ctx, options, report)
	}()
	return <-completed
}

func captureOnDedicatedThread(ctx context.Context, options lab.CaptureOptions, report func(lab.CaptureSnapshot)) error {
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)

	counters := &captureCounters{}
	keyboardCallback := syscall.NewCallback(func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			data := (*keyboardHookData)(unsafe.Pointer(lParam))
			counters.observeKeyboard(uint32(wParam), data.Flags&llkhfInjected != 0)
		}
		result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
		return result
	})
	mouseCallback := syscall.NewCallback(func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			data := (*mouseHookData)(unsafe.Pointer(lParam))
			counters.observeMouse(uint32(wParam), data.Flags&llmhfInjected != 0, data.ExtraInfo)
		}
		result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
		return result
	})

	module, _, callErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		return fmt.Errorf("GetModuleHandleW failed: %w", callErr)
	}
	keyboardHook, _, callErr := procSetWindowsHookExW.Call(whKeyboardLL, keyboardCallback, module, 0)
	if keyboardHook == 0 {
		return fmt.Errorf("SetWindowsHookExW(WH_KEYBOARD_LL) failed: %w", callErr)
	}

	mouseHook, _, callErr := procSetWindowsHookExW.Call(whMouseLL, mouseCallback, module, 0)
	if mouseHook == 0 {
		procUnhookWindowsHookEx.Call(keyboardHook)
		return fmt.Errorf("SetWindowsHookExW(WH_MOUSE_LL) failed: %w", callErr)
	}
	cleanupHooks := func() error {
		var firstErr error
		if mouseHook != 0 {
			result, _, unhookErr := procUnhookWindowsHookEx.Call(mouseHook)
			mouseHook = 0
			if result == 0 {
				firstErr = fmt.Errorf("UnhookWindowsHookEx(WH_MOUSE_LL) failed: %w", unhookErr)
			}
		}
		if keyboardHook != 0 {
			result, _, unhookErr := procUnhookWindowsHookEx.Call(keyboardHook)
			keyboardHook = 0
			if result == 0 && firstErr == nil {
				firstErr = fmt.Errorf("UnhookWindowsHookEx(WH_KEYBOARD_LL) failed: %w", unhookErr)
			}
		}
		return firstErr
	}
	defer cleanupHooks()
	if options.Ready != nil {
		result, _, postErr := procPostThreadMessageW.Call(threadID, wmCaptureReady, 0, 0)
		if result == 0 {
			return fmt.Errorf("PostThreadMessageW(capture ready) failed: %w", postErr)
		}
	}

	started := time.Now()
	stopReporter := make(chan struct{})
	reporterDone := make(chan struct{})
	go func() {
		defer close(reporterDone)
		ticker := time.NewTicker(options.ReportInterval)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				report(counters.snapshot(now.Sub(started)))
			case <-stopReporter:
				return
			}
		}
	}()

	stopMessageLoop := make(chan struct{})
	stopMessageLoopDone := make(chan struct{})
	go func() {
		defer close(stopMessageLoopDone)
		select {
		case <-ctx.Done():
			procPostThreadMessageW.Call(threadID, wmQuit, 0, 0)
		case <-stopMessageLoop:
		}
	}()

	loopErr := runMessageLoop(options.Ready)
	unhookErr := cleanupHooks()
	close(stopMessageLoop)
	<-stopMessageLoopDone
	close(stopReporter)
	<-reporterDone
	report(counters.snapshot(time.Since(started)))

	if loopErr != nil {
		return loopErr
	}
	if unhookErr != nil {
		return unhookErr
	}
	return ctx.Err()
}

func runMessageLoop(ready func()) error {
	var msg message
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		signedResult := int32(result)
		switch signedResult {
		case -1:
			return fmt.Errorf("GetMessageW failed: %w", callErr)
		case 0:
			return nil
		default:
			if msg.Message == wmCaptureReady {
				if ready != nil {
					ready()
					ready = nil
				}
				continue
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func (c *captureCounters) observeKeyboard(message uint32, injected bool) {
	switch message {
	case wmKeyDown, wmSysKeyDown:
		if injected {
			c.keyboardInjectedDown.Add(1)
		} else {
			c.keyboardPhysicalDown.Add(1)
		}
	case wmKeyUp, wmSysKeyUp:
		if injected {
			c.keyboardInjectedUp.Add(1)
		} else {
			c.keyboardPhysicalUp.Add(1)
		}
	default:
		c.keyboardOther.Add(1)
	}
}

func (c *captureCounters) observeMouse(message uint32, injected bool, extraInfo uintptr) {
	switch message {
	case wmMouseMove:
		if injected {
			c.mouseInjectedMove.Add(1)
			if extraInfo == selfTestExtraInfo {
				c.mouseSelfTestInjectedMove.Add(1)
			}
		} else {
			c.mousePhysicalMove.Add(1)
		}
	case wmLButtonDown, wmRButtonDown, wmMButtonDown, wmXButtonDown:
		if injected {
			c.mouseInjectedButtonDown.Add(1)
		} else {
			c.mousePhysicalButtonDown.Add(1)
		}
	case wmLButtonUp, wmRButtonUp, wmMButtonUp, wmXButtonUp:
		if injected {
			c.mouseInjectedButtonUp.Add(1)
		} else {
			c.mousePhysicalButtonUp.Add(1)
		}
	case wmMouseWheel, wmMouseHWheel:
		if injected {
			c.mouseInjectedWheel.Add(1)
		} else {
			c.mousePhysicalWheel.Add(1)
		}
	default:
		c.mouseOther.Add(1)
	}
}

func (c *captureCounters) snapshot(elapsed time.Duration) lab.CaptureSnapshot {
	return lab.CaptureSnapshot{
		SchemaVersion: lab.CaptureSchemaVersion,
		Elapsed:       elapsed,
		ElapsedMillis: elapsed.Milliseconds(),
		Keyboard: lab.EventCounts{
			PhysicalDown: c.keyboardPhysicalDown.Load(),
			PhysicalUp:   c.keyboardPhysicalUp.Load(),
			InjectedDown: c.keyboardInjectedDown.Load(),
			InjectedUp:   c.keyboardInjectedUp.Load(),
			Other:        c.keyboardOther.Load(),
		},
		Mouse: lab.MouseCounts{
			PhysicalMove:         c.mousePhysicalMove.Load(),
			InjectedMove:         c.mouseInjectedMove.Load(),
			PhysicalButtonDown:   c.mousePhysicalButtonDown.Load(),
			PhysicalButtonUp:     c.mousePhysicalButtonUp.Load(),
			InjectedButtonDown:   c.mouseInjectedButtonDown.Load(),
			InjectedButtonUp:     c.mouseInjectedButtonUp.Load(),
			PhysicalWheel:        c.mousePhysicalWheel.Load(),
			InjectedWheel:        c.mouseInjectedWheel.Load(),
			Other:                c.mouseOther.Load(),
			SelfTestInjectedMove: c.mouseSelfTestInjectedMove.Load(),
		},
	}
}
