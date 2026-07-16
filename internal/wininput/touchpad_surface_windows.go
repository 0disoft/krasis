//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/lab"
)

const (
	wmDestroy                 = 0x0002
	wmActivate                = 0x0006
	wmClose                   = 0x0010
	wmPointerUpdate           = 0x0245
	wmPointerDown             = 0x0246
	wmPointerUp               = 0x0247
	wmPointerEnter            = 0x0249
	wmPointerLeave            = 0x024A
	wmPointerCaptureChanged   = 0x024C
	wmPointerWheel            = 0x024E
	wmPointerHorizontalWheel  = 0x024F
	wmTouchpadSurfaceStop     = 0x8024
	ptTouchpad                = 5
	waInactive                = 0
	wsOverlappedWindow        = 0x00CF0000
	wsVisible                 = 0x10000000
	swShow                    = 5
	idcArrow                  = 32512
	colorWindowBrush          = 6
	registerTouchpadWindowOrd = 2689
	touchpadStopDuration      = 1
	touchpadStopCanceled      = 2
	touchpadStopWindowClosed  = 3
	touchpadStopEscape        = 4
	touchpadProbeWindowWidth  = 720
	touchpadProbeWindowHeight = 420
)

var (
	procRegisterClassWTouchpad   = user32.NewProc("RegisterClassW")
	procUnregisterClassWTouchpad = user32.NewProc("UnregisterClassW")
	procDefWindowProcWTouchpad   = user32.NewProc("DefWindowProcW")
	procPostMessageWTouchpad     = user32.NewProc("PostMessageW")
	procPostQuitMessageTouchpad  = user32.NewProc("PostQuitMessage")
	procShowWindowTouchpad       = user32.NewProc("ShowWindow")
	procUpdateWindowTouchpad     = user32.NewProc("UpdateWindow")
	procSetForegroundTouchpad    = user32.NewProc("SetForegroundWindow")
	procGetForegroundTouchpad    = user32.NewProc("GetForegroundWindow")
	procLoadCursorWTouchpad      = user32.NewProc("LoadCursorW")
	procGetPointerTypeTouchpad   = user32.NewProc("GetPointerType")
	procGetProcAddressTouchpad   = kernel32.NewProc("GetProcAddress")

	touchpadSurfaceClassSequence atomic.Uint64
)

type touchpadSurfaceWindowClass struct {
	Style       uint32
	WindowProc  uintptr
	ClassExtra  int32
	WindowExtra int32
	Instance    uintptr
	Icon        uintptr
	Cursor      uintptr
	Background  uintptr
	MenuName    *uint16
	ClassName   *uint16
}

type touchpadSurfaceCounters struct {
	activations       uint64
	deactivations     uint64
	down              uint64
	update            uint64
	up                uint64
	enter             uint64
	leave             uint64
	captureChanged    uint64
	wheel             uint64
	horizontalWheel   uint64
	typeQueries       uint64
	typeQueryFailures uint64
	touchpadConfirmed uint64
	otherConfirmed    uint64
	legacyVertical    uint64
	legacyHorizontal  uint64
}

func (platform *Platform) TouchpadSurface(ctx context.Context, options lab.TouchpadSurfaceOptions) (lab.TouchpadSurfaceObservation, error) {
	if platform == nil {
		return lab.TouchpadSurfaceObservation{}, errors.New("touchpad surface platform is not initialized")
	}
	if ctx == nil {
		return lab.TouchpadSurfaceObservation{}, errors.New("touchpad surface context is required")
	}
	if options.Duration <= 0 {
		return lab.TouchpadSurfaceObservation{}, errors.New("touchpad surface duration must be positive")
	}
	if err := ctx.Err(); err != nil {
		return lab.TouchpadSurfaceObservation{}, err
	}

	type result struct {
		observation lab.TouchpadSurfaceObservation
		err         error
	}
	completed := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		observation, err := touchpadSurfaceOnDedicatedThread(ctx, options)
		completed <- result{observation: observation, err: err}
	}()
	finished := <-completed
	return finished.observation, finished.err
}

func touchpadSurfaceOnDedicatedThread(ctx context.Context, options lab.TouchpadSurfaceOptions) (observation lab.TouchpadSurfaceObservation, resultErr error) {
	registerProc, registerAvailable := resolveRegisterTouchpadCapableWindow()
	pointerTypeAvailable := procGetPointerTypeTouchpad.Find() == nil
	observation.API.RegisterAvailable = registerAvailable
	observation.API.PointerTypeAvailable = pointerTypeAvailable
	if !registerAvailable || !pointerTypeAvailable {
		return observation, errors.Join(lab.ErrUnsupported, errors.New("Windows 11 Precision Touchpad pointer APIs are unavailable"))
	}

	module, _, callErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		return observation, touchpadWin32Error("GetModuleHandleW", callErr)
	}
	className, err := syscall.UTF16PtrFromString(fmt.Sprintf("KrasisTouchpadSurface-%d-%d", os.Getpid(), touchpadSurfaceClassSequence.Add(1)))
	if err != nil {
		return observation, fmt.Errorf("encode touchpad surface class name: %w", err)
	}
	windowTitle, err := syscall.UTF16PtrFromString("Krasis touchpad surface - keep this window active and use two-finger scrolling")
	if err != nil {
		return observation, fmt.Errorf("encode touchpad surface window title: %w", err)
	}

	counters := &touchpadSurfaceCounters{}
	var stopCode atomic.Int32
	windowDestroyed := false
	wndProc := syscall.NewCallback(func(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
		switch message {
		case wmActivate:
			if uint16(wParam) == waInactive {
				counters.deactivations++
			} else {
				counters.activations++
			}
		case wmPointerDown, wmPointerUpdate, wmPointerUp, wmPointerEnter, wmPointerLeave,
			wmPointerCaptureChanged, wmPointerWheel, wmPointerHorizontalWheel:
			counters.observePointer(message, uint32(wParam&0xFFFF), queryPointerType)
		case wmMouseWheel:
			counters.legacyVertical++
		case wmMouseHWheel:
			counters.legacyHorizontal++
		case wmKeyDown:
			if wParam == 0x1B {
				stopCode.CompareAndSwap(0, touchpadStopEscape)
				procPostQuitMessageTouchpad.Call(0)
				return 0
			}
		case wmClose:
			stopCode.CompareAndSwap(0, touchpadStopWindowClosed)
			procPostQuitMessageTouchpad.Call(0)
			return 0
		case wmTouchpadSurfaceStop:
			procPostQuitMessageTouchpad.Call(0)
			return 0
		case wmDestroy:
			windowDestroyed = true
			procPostQuitMessageTouchpad.Call(0)
			return 0
		}
		result, _, _ := procDefWindowProcWTouchpad.Call(window, uintptr(message), wParam, lParam)
		return result
	})
	cursor, _, _ := procLoadCursorWTouchpad.Call(0, idcArrow)
	class := touchpadSurfaceWindowClass{
		WindowProc: wndProc,
		Instance:   module,
		Cursor:     cursor,
		Background: colorWindowBrush,
		ClassName:  className,
	}
	atom, _, callErr := procRegisterClassWTouchpad.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return observation, touchpadWin32Error("RegisterClassW(touchpad surface)", callErr)
	}
	defer func() {
		if unregistered, _, unregisterErr := procUnregisterClassWTouchpad.Call(uintptr(unsafe.Pointer(className)), module); unregistered == 0 {
			resultErr = errors.Join(resultErr, touchpadWin32Error("UnregisterClassW(touchpad surface)", unregisterErr))
		}
		runtime.KeepAlive(wndProc)
	}()

	x := (int(systemMetric(smCXVIRTUALSCREEN))-touchpadProbeWindowWidth)/2 + int(systemMetric(smXVIRTUALSCREEN))
	y := (int(systemMetric(smCYVIRTUALSCREEN))-touchpadProbeWindowHeight)/2 + int(systemMetric(smYVIRTUALSCREEN))
	window, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		wsOverlappedWindow|wsVisible,
		uintptr(x), uintptr(y), touchpadProbeWindowWidth, touchpadProbeWindowHeight,
		0, 0, module, 0,
	)
	if window == 0 {
		return observation, touchpadWin32Error("CreateWindowExW(touchpad surface)", callErr)
	}
	observation.Window.Created = true
	registered := false
	defer func() {
		if registered {
			if result, _, unregisterErr := syscall.SyscallN(registerProc, window, 0); result == 0 {
				resultErr = errors.Join(resultErr, touchpadWin32Error("RegisterTouchpadCapableWindow(FALSE)", unregisterErr))
			}
		}
		if !windowDestroyed {
			if destroyed, _, destroyErr := procDestroyWindow.Call(window); destroyed == 0 {
				resultErr = errors.Join(resultErr, touchpadWin32Error("DestroyWindow(touchpad surface)", destroyErr))
			}
		}
	}()

	if result, _, registerErr := syscall.SyscallN(registerProc, window, 1); result == 0 {
		return observation, touchpadWin32Error("RegisterTouchpadCapableWindow(TRUE)", registerErr)
	}
	registered = true
	observation.API.Registered = true
	procShowWindowTouchpad.Call(window, swShow)
	procUpdateWindowTouchpad.Call(window)
	procSetForegroundTouchpad.Call(window)
	if foreground, _, _ := procGetForegroundTouchpad.Call(); foreground == window {
		observation.Window.ForegroundObserved = true
	}
	if options.Ready != nil {
		options.Ready()
	}

	started := time.Now()
	stopWatcherDone := make(chan struct{})
	stopWatcher := make(chan struct{})
	go func() {
		defer close(stopWatcherDone)
		timer := time.NewTimer(options.Duration)
		defer timer.Stop()
		select {
		case <-timer.C:
			stopCode.CompareAndSwap(0, touchpadStopDuration)
		case <-ctx.Done():
			stopCode.CompareAndSwap(0, touchpadStopCanceled)
		case <-stopWatcher:
			return
		}
		procPostMessageWTouchpad.Call(window, wmTouchpadSurfaceStop, 0, 0)
	}()

	loopErr := touchpadSurfaceMessageLoop()
	close(stopWatcher)
	<-stopWatcherDone
	observation.ElapsedMillis = time.Since(started).Milliseconds()
	observation.Window.Activations = counters.activations
	observation.Window.Deactivations = counters.deactivations
	if !observation.Window.ForegroundObserved {
		observation.Window.ForegroundObserved = counters.activations > 0
	}
	observation.Pointer = counters.pointerSnapshot()
	observation.Legacy = lab.TouchpadSurfaceLegacyCounts{
		VerticalWheels:   counters.legacyVertical,
		HorizontalWheels: counters.legacyHorizontal,
	}
	observation.StopReason = touchpadSurfaceStopReason(stopCode.Load())
	if loopErr != nil {
		return observation, loopErr
	}
	if stopCode.Load() == touchpadStopCanceled {
		return observation, ctx.Err()
	}
	return observation, nil
}

func (counters *touchpadSurfaceCounters) observePointer(message uint32, pointerID uint32, query func(uint32) (uint32, bool)) {
	switch message {
	case wmPointerDown:
		counters.down++
	case wmPointerUpdate:
		counters.update++
	case wmPointerUp:
		counters.up++
	case wmPointerEnter:
		counters.enter++
	case wmPointerLeave:
		counters.leave++
	case wmPointerCaptureChanged:
		counters.captureChanged++
	case wmPointerWheel:
		counters.wheel++
	case wmPointerHorizontalWheel:
		counters.horizontalWheel++
	}
	counters.typeQueries++
	pointerType, ok := query(pointerID)
	if !ok {
		counters.typeQueryFailures++
		return
	}
	if pointerType == ptTouchpad {
		counters.touchpadConfirmed++
	} else {
		counters.otherConfirmed++
	}
}

func queryPointerType(pointerID uint32) (uint32, bool) {
	var pointerType uint32
	result, _, _ := syscall.SyscallN(
		procGetPointerTypeTouchpad.Addr(),
		uintptr(pointerID),
		uintptr(unsafe.Pointer(&pointerType)),
	)
	return pointerType, result != 0
}

func (counters *touchpadSurfaceCounters) pointerSnapshot() lab.TouchpadSurfacePointerCounts {
	return lab.TouchpadSurfacePointerCounts{
		Down:              counters.down,
		Update:            counters.update,
		Up:                counters.up,
		Enter:             counters.enter,
		Leave:             counters.leave,
		CaptureChanged:    counters.captureChanged,
		Wheel:             counters.wheel,
		HorizontalWheel:   counters.horizontalWheel,
		TypeQueries:       counters.typeQueries,
		TypeQueryFailures: counters.typeQueryFailures,
		TouchpadConfirmed: counters.touchpadConfirmed,
		OtherConfirmed:    counters.otherConfirmed,
	}
}

func touchpadSurfaceMessageLoop() error {
	var current message
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&current)), 0, 0, 0)
		switch int32(result) {
		case -1:
			return touchpadWin32Error("GetMessageW(touchpad surface)", callErr)
		case 0:
			return nil
		default:
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&current)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&current)))
		}
	}
}

func resolveRegisterTouchpadCapableWindow() (uintptr, bool) {
	named := user32.NewProc("RegisterTouchpadCapableWindow")
	if named.Find() == nil {
		return named.Addr(), true
	}
	user32Name, err := syscall.UTF16PtrFromString("user32.dll")
	if err != nil {
		return 0, false
	}
	module, _, _ := procGetModuleHandleW.Call(uintptr(unsafe.Pointer(user32Name)))
	if module == 0 {
		return 0, false
	}
	address, _, _ := procGetProcAddressTouchpad.Call(module, registerTouchpadWindowOrd)
	return address, address != 0
}

func touchpadSurfaceStopReason(code int32) string {
	switch code {
	case touchpadStopDuration:
		return "duration_elapsed"
	case touchpadStopCanceled:
		return "canceled"
	case touchpadStopWindowClosed:
		return "window_closed"
	case touchpadStopEscape:
		return "escape"
	default:
		return "message_loop_stopped"
	}
}

func touchpadWin32Error(operation string, err error) error {
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return errors.New(operation + " failed")
	}
	return fmt.Errorf("%s failed: %w", operation, err)
}
