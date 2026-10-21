//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	wmReturnCursorShow     = 0x8040
	wmReturnCursorHide     = 0x8041
	wmReturnCursorSnapshot = 0x8042
)

// The window and low-level hooks share one OS thread. Physical input hides the
// passive shield synchronously before CallNextHookEx, without swallowing input.
type returnCursorShield struct {
	report          SourcePresentationReporter // Configured before the native owner starts.
	loginContext    context.Context            // Only inspected on the native owner thread.
	window          atomic.Uintptr
	generation      atomic.Uint64
	snapshotID      atomic.Uint64
	snapshotRequest atomic.Pointer[returnCursorSnapshotRequest]
	mu              sync.Mutex
	position        point
	visible         bool // accessed only on the owning hook thread
	failure         atomic.Pointer[returnCursorFailure]
}

type returnCursorFailure struct{ err error }

func (shield *returnCursorShield) record(stage string, err error) {
	if err != nil {
		shield.failure.CompareAndSwap(nil, &returnCursorFailure{sourceStage(stage, err)})
	}
}
func (shield *returnCursorShield) failureResult() error {
	if value := shield.failure.Load(); value != nil {
		return value.err
	}
	return nil
}

var (
	returnCursorClassOnce    sync.Once
	returnCursorClassName, _ = syscall.UTF16PtrFromString("KrasisReturnCursorShield")
	returnCursorClassOK      bool
	returnCursorWindows      sync.Map
	returnCursorSet          = user32.NewProc("SetCursor")
	returnCursorVisible      = user32.NewProc("IsWindowVisible")
	returnCursorDestroy      = func(window uintptr) error {
		if ok, _, err := procDestroyWindow.Call(window); ok == 0 {
			return fmt.Errorf("DestroyWindow(return cursor): %w", err)
		}
		return nil
	}
	returnCursorHideWindow = func(window uintptr) error {
		procShowWindowTouchpad.Call(window, swHide)
		if visible, _, _ := returnCursorVisible.Call(window); visible != 0 {
			return errors.New("return cursor window remained visible")
		}
		return nil
	}
	returnCursorPost = func(window uintptr, message uint32, generation uint64) error {
		if ok, _, err := procPostMessageWTouchpad.Call(window, uintptr(message), uintptr(generation), 0); ok == 0 {
			return fmt.Errorf("PostMessageW(return cursor): %w", err)
		}
		return nil
	}
	returnCursorShowWindow = func(window uintptr, p point) error {
		// Nonzero alpha preserves hit testing; no activation, capture or focus.
		if ok, _, err := procSetWindowPosSourceCapture.Call(window, ^uintptr(0), uintptr(uint32(p.X-12)), uintptr(uint32(p.Y-12)), 24, 24, 0x10|0x40); ok == 0 {
			return fmt.Errorf("SetWindowPos(return cursor): %w", err)
		}
		returnCursorSet.Call(0)
		if visible, _, _ := returnCursorVisible.Call(window); visible == 0 {
			return errors.New("return cursor window was not visible")
		}
		return nil
	}
	returnCursorPosition = cursorPosition
	returnCursorWndProc  = syscall.NewCallback(func(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
		if value, ok := returnCursorWindows.Load(window); ok {
			shield := value.(*returnCursorShield)
			switch message {
			case wmReturnCursorSnapshot:
				shield.snapshotPosition(uint64(wParam))
				return 0
			case wmReturnCursorShow:
				shield.show(uint64(wParam))
				return 0
			case wmReturnCursorHide:
				if shield.generation.Load() == uint64(wParam) {
					shield.hide()
				}
				return 0
			case 0x20: // WM_SETCURSOR
				returnCursorSet.Call(0)
				return 1
			case 0x21: // WM_MOUSEACTIVATE: never take keyboard focus
				return 3
			}
		}
		result, _, _ := procDefWindowProcWTouchpad.Call(window, uintptr(message), wParam, lParam)
		return result
	})
)

func (hook *SourceHook) PrepareReturnCursor() func() { return hook.returnCursor.prepare() }
func (hook *SourceHook) RestoreReturnCursor()        { hook.returnCursor.restore() }

func (shield *returnCursorShield) prepare() func() {
	generation := shield.generation.Load()
	position, ok := shield.requestPosition(generation)
	if !ok {
		return nil
	}
	return func() {
		shield.mu.Lock()
		if shield.generation.Load() != generation {
			shield.mu.Unlock()
			return
		}
		shield.position = position
		shield.mu.Unlock()
		if window := shield.window.Load(); window != 0 {
			shield.record("cursor-present", returnCursorPost(window, wmReturnCursorShow, generation))
		}
	}
}

func (shield *returnCursorShield) show(generation uint64) {
	started, result := time.Now(), "unavailable"
	defer func() { shield.observe("return_show", result, started) }()
	if sourceLoginDesktopCheck(shield.loginContext) != nil {
		result = "inactive"
		return
	}
	shield.mu.Lock()
	expectedPosition := shield.position
	shield.mu.Unlock()
	if shield.generation.Load() != generation {
		result = "superseded"
		return
	}
	position, err := returnCursorPosition()
	if err != nil || position != expectedPosition {
		result = "moved"
		if err != nil {
			result = "failed"
		}
		return
	}
	// Native queries/window calls can dispatch this thread's hook again. Input
	// invalidation must win even when the cursor stayed at the same position.
	if shield.generation.Load() != generation {
		result = "superseded"
		return
	}
	if window := shield.window.Load(); window != 0 {
		shield.visible = true
		err := returnCursorShowWindow(window, position)
		shield.record("cursor-present", err)
		if shield.generation.Load() != generation {
			// A nested hook may have hidden the window before the outer show
			// completed. Its false ledger alone cannot prove it stayed hidden.
			if shield.window.Load() == window {
				shield.visible = true
				shield.hide()
			}
			result = "superseded"
			if shield.failureResult() != nil {
				result = "failed"
			}
			return
		}
		result = "accepted"
		if err != nil {
			result = "failed"
		}
	}
}

func (shield *returnCursorShield) observe(operation, result string, started time.Time) {
	if shield.report != nil {
		shield.report(operation, result, time.Since(started))
	}
}

func (shield *returnCursorShield) hide() {
	if shield.visible {
		started, result := time.Now(), "accepted"
		// A native hide can itself reenter the hook. Publish the in-progress
		// hide before that call so nested input cannot recursively hide forever.
		shield.visible = false
		if err := returnCursorHideWindow(shield.window.Load()); err != nil {
			shield.visible = true
			shield.record("cursor-restore", err)
			result = "failed"
		}
		shield.observe("return_hide", result, started)
	}
}

func (shield *returnCursorShield) beforeInput() error {
	if shield == nil {
		return nil
	}
	shield.generation.Add(1)
	shield.hide()
	return shield.failureResult()
}

// Ignored injected input is not physical capture work. With no visible shield,
// only invalidate queued presentation and preserve earlier cleanup failures;
// no native operation, desktop selection or input authority is needed. A visible
// shield still requires the same current-desktop check before native hiding.
// Both this and beforeInput run only on the owning low-level hook thread.
func (shield *returnCursorShield) beforeIgnoredInput(check func() error) error {
	if shield != nil && shield.visible {
		if err := check(); err != nil {
			return err
		}
	}
	return shield.beforeInput()
}

func (shield *returnCursorShield) restore() {
	if shield == nil {
		return
	}
	generation := shield.generation.Add(1)
	if window := shield.window.Load(); window != 0 {
		shield.record("cursor-restore", returnCursorPost(window, wmReturnCursorHide, generation))
	}
}

func (shield *returnCursorShield) open() {
	module, _, _ := procGetModuleHandleW.Call(0)
	if module == 0 {
		return
	}
	returnCursorClassOnce.Do(func() {
		class := touchpadSurfaceWindowClass{WindowProc: returnCursorWndProc, Instance: module, Background: colorWindowBrush, ClassName: returnCursorClassName}
		atom, _, _ := procRegisterClassWTouchpad.Call(uintptr(unsafe.Pointer(&class)))
		returnCursorClassOK = atom != 0
	})
	if !returnCursorClassOK {
		return
	}
	window, _, _ := procCreateWindowExW.Call(wsExTopmost|wsExToolWindow|wsExLayeredSourceCapture|0x08000000, uintptr(unsafe.Pointer(returnCursorClassName)), 0, wsPopup, 0, 0, 24, 24, 0, 0, module, 0)
	if window == 0 {
		return
	}
	returnCursorWindows.Store(window, shield)
	shield.generation.Add(1)
	shield.window.Store(window)
	if err := configureSourceWheelCaptureAppearance(window); err != nil {
		_ = shield.close() // Preserve any failed native cleanup for the worker result.
		return
	}
}

func (shield *returnCursorShield) close() error {
	shield.beforeInput()
	window := shield.window.Load()
	if window != 0 {
		if err := returnCursorDestroy(window); err != nil {
			shield.record("cursor-release", err)
		} else {
			returnCursorWindows.Delete(window)
			shield.window.Store(0)
			shield.visible = false
		}
	}
	return shield.failureResult()
}
