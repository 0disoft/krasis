//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/lab"
)

const (
	wmSourceWheelCaptureActivate   = 0x8025
	wmSourceWheelCaptureDeactivate = 0x8026
	wmSourceWheelCaptureStop       = 0x8027
	wsPopup                        = 0x80000000
	wsExTopmost                    = 0x00000008
	wsExToolWindow                 = 0x00000080
	wsExLayeredSourceCapture       = 0x00080000
	sourceWheelCaptureAlpha        = 1
	sourceWheelCaptureAlphaFlag    = 0x00000002
	swHide                         = 0
	swShowWithoutActivation        = 8 // SW_SHOWNA preserves current size and position.
	swpNoActivate                  = 0x0010
	sourceWheelCaptureWidth        = 24
	sourceWheelCaptureHeight       = 24
	sourceWheelCaptureCallTimeout  = 2 * time.Second
)

var (
	callSourceAttachThreadInput       = user32.NewProc("AttachThreadInput").Call
	callSourceForegroundProbe         = user32.NewProc("SendMessageTimeoutW").Call
	sourceWheelCaptureProbeForeground = func(window uintptr) error {
		// WM_NULL needs only the API success flag, not the optional result.
		// Avoid passing a Go pointer through the injectable syscall adapter.
		sent, _, callErr := callSourceForegroundProbe(window, 0, 0, 0, 0x23, 100, 0)
		if sent == 0 {
			return touchpadWin32Error("SendMessageTimeoutW(source foreground probe)", callErr)
		}
		return nil
	}
	callSetSourceCaptureCursor         = user32.NewProc("SetCursor").Call
	callSetLayeredSourceCapture        = user32.NewProc("SetLayeredWindowAttributes").Call
	procSetWindowPosSourceCapture      = user32.NewProc("SetWindowPos")
	procIsWindowSourceCapture          = user32.NewProc("IsWindow")
	procGetWindowThreadIDSourceCapture = user32.NewProc("GetWindowThreadProcessId")
	sourceWheelCaptureShowWindow       = func(window uintptr, command uintptr) {
		procShowWindowTouchpad.Call(window, command)
	}
	sourceWheelCaptureGetForeground = func() uintptr {
		foreground, _, _ := procGetForegroundTouchpad.Call()
		return foreground
	}
	sourceWheelCaptureSetForeground = func(window uintptr) error {
		foreground, _, foregroundErr := procSetForegroundTouchpad.Call(window)
		if foreground == 0 {
			return touchpadWin32Error("SetForegroundWindow(source wheel capture)", foregroundErr)
		}
		return nil
	}
	sourceWheelCaptureCurrentThread = func() uint32 {
		thread, _, _ := procGetCurrentThreadID.Call()
		return uint32(thread)
	}
	sourceWheelCaptureWindowThread = func(window uintptr) (uint32, error) {
		thread, _, callErr := procGetWindowThreadIDSourceCapture.Call(window, 0)
		if thread != 0 {
			return uint32(thread), nil
		}
		if callErr == syscall.Errno(0) {
			return 0, errors.New("GetWindowThreadProcessId returned no extended error")
		}
		return 0, callErr
	}
	sourceWheelCaptureAttachThreads = func(sourceThread, targetThread uint32, attach bool) error {
		value := uintptr(0)
		if attach {
			value = 1
		}
		attached, _, callErr := callSourceAttachThreadInput(uintptr(sourceThread), uintptr(targetThread), value)
		if attached == 0 {
			return touchpadWin32Error("AttachThreadInput(source foreground)", callErr)
		}
		return nil
	}
	sourceWheelCaptureIsWindow = func(window uintptr) bool {
		valid, _, _ := procIsWindowSourceCapture.Call(window)
		return valid != 0
	}
	sourceWheelCaptureRegisterWindow = func(proc, window uintptr, enabled bool) error {
		value := uintptr(0)
		if enabled {
			value = 1
		}
		registered, _, registerErr := syscall.SyscallN(proc, window, value)
		if registered == 0 {
			return registerErr
		}
		return nil
	}
	sourceWheelCaptureSetPosition = func(window uintptr, x, y int32) error {
		topmost := ^uintptr(0)
		positioned, _, positionErr := procSetWindowPosSourceCapture.Call(
			window, topmost, uintptr(x), uintptr(y), sourceWheelCaptureWidth, sourceWheelCaptureHeight, swpNoActivate,
		)
		if positioned == 0 {
			return touchpadWin32Error("SetWindowPos(source wheel capture)", positionErr)
		}
		return nil
	}

	sourceWheelCaptureClassSequence atomic.Uint64
	sourceWheelCaptureTimeout       = sourceWheelCaptureCallTimeout
)

var (
	ErrSourceCaptureLostForeground             = errors.New("source wheel capture lost foreground")
	ErrSourceCaptureUnregisterFailed           = errors.New("source wheel capture unregister failed")
	ErrSourceCaptureCommandTimeout             = errors.New("source wheel capture command timed out")
	errSourceWheelCaptureInactive              = errors.New("source wheel capture received input outside an active source session")
	ErrSourceCaptureActivationSetup            = errors.New("source capture activation setup failed")
	ErrSourceCaptureActivationAPIRejected      = errors.New("source capture foreground activation was rejected")
	ErrSourceCaptureActivationVerificationLost = errors.New("source capture foreground activation lost verification")
	ErrSourceCaptureThreadAttachFailed         = errors.New("source capture foreground thread attach failed")
	ErrSourceCaptureThreadDetachFailed         = errors.New("source capture foreground thread detach failed")
	ErrSourceFocusRestoreAPIRejected           = errors.New("source focus restore was rejected")
	ErrSourceFocusRestoreVerificationLost      = errors.New("source focus restore lost verification")
	ErrSourceFocusRestoreThreadAttachFailed    = errors.New("source focus restore thread attach failed")
	ErrSourceFocusRestoreThreadDetachFailed    = errors.New("source focus restore thread detach failed")
	ErrSourceFocusReleaseCaptureRetained       = errors.New("source capture window retained foreground after hide")
)

type foregroundThreadFallbackResult uint8

const (
	foregroundThreadFallbackSucceeded foregroundThreadFallbackResult = iota + 1
	foregroundThreadFallbackAttachFailed
	foregroundThreadFallbackDetachFailed
	foregroundThreadFallbackAPIRejected
	foregroundThreadFallbackVerificationLost
)

type sourceWheelCapture struct {
	observer SourcePipelineObserver
	opOnce   sync.Once
	opGate   chan struct{}
	runMu    sync.RWMutex
	run      *sourceWheelCaptureRun
}

type sourceWheelCaptureRun struct {
	attachedSource, attachedPeer uint32 // Native owner thread only; retained until detach succeeds.
	shutdownEvidence             *sourceShutdownEvidence
	window                       uintptr
	commands                     chan sourceWheelCaptureCommand
	done                         chan struct{}
	active                       atomic.Bool
	terminalMu                   sync.Mutex
	terminalErr                  error
	registerProc                 uintptr
	savedFocus                   uintptr
	registered                   bool
	fullscreenMarked             bool // Capture owner thread only; presentation, not input authority.
	session                      *SourceSession
	observer                     SourcePipelineObserver
}

type sourceWheelCaptureCommand struct {
	message  uint32
	result   chan error
	profile  context.Context
	queuedAt time.Time
}

func newSourceWheelCapture(observer SourcePipelineObserver) *sourceWheelCapture {
	return &sourceWheelCapture{observer: observer}
}

func (capture *sourceWheelCapture) Activate() error {
	return capture.ActivateProfile(nil)
}

func (capture *sourceWheelCapture) ActivateProfile(profile context.Context) error {
	if capture == nil {
		return errors.New("source wheel capture is not initialized")
	}
	return capture.executeProfile(profile, wmSourceWheelCaptureActivate, false)
}

func (capture *sourceWheelCapture) Deactivate() {
	capture.DeactivateProfile(nil)
}

func (capture *sourceWheelCapture) DeactivateProfile(profile context.Context) {
	if capture == nil {
		return
	}
	if err := capture.executeProfile(profile, wmSourceWheelCaptureDeactivate, true); err != nil {
		capture.failCurrentRun(err)
	}
}

func (capture *sourceWheelCapture) Active() bool {
	if capture == nil {
		return false
	}
	capture.runMu.RLock()
	current := capture.run
	capture.runMu.RUnlock()
	return current != nil && current.active.Load()
}

func (capture *sourceWheelCapture) OwnsWheel() bool {
	if capture == nil {
		return false
	}
	capture.runMu.RLock()
	current := capture.run
	capture.runMu.RUnlock()
	if current == nil || !current.active.Load() || current.window == 0 {
		return false
	}
	return sourceWheelCaptureGetForeground() == current.window
}

func (capture *sourceWheelCapture) execute(message uint32, allowMissing bool) error {
	return capture.executeProfile(nil, message, allowMissing)
}

func (capture *sourceWheelCapture) executeProfile(profile context.Context, message uint32, allowMissing bool) error {
	stopTotal := handoff.CaptureSpan(profile, handoff.CaptureTotal)
	defer stopTotal()
	timer := time.NewTimer(sourceWheelCaptureTimeout)
	defer timer.Stop()
	capture.opOnce.Do(func() { capture.opGate = make(chan struct{}, 1) })
	stopSerialize := handoff.CaptureSpan(profile, handoff.CaptureSerializeWait)
	select {
	case capture.opGate <- struct{}{}:
		defer func() { <-capture.opGate }()
	case <-timer.C:
		stopSerialize()
		err := fmt.Errorf("source wheel capture command serialization timed out: %w", ErrSourceCaptureCommandTimeout)
		capture.failCurrentRun(err)
		return err
	}
	stopSerialize()
	capture.runMu.RLock()
	current := capture.run
	capture.runMu.RUnlock()
	if current == nil {
		if allowMissing {
			return nil
		}
		return errors.New("source wheel capture window is not ready")
	}
	if terminalErr := current.err(); terminalErr != nil {
		// Context cancellation is an expected worker result during shutdown.
		// Adding an independent error would make cancellationOnly classify it
		// as a native cleanup failure and latch an otherwise clean stop FAULTED.
		return fmt.Errorf("source wheel capture window is terminal: %w", terminalErr)
	}
	command := sourceWheelCaptureCommand{message: message, result: make(chan error, 1), profile: profile}
	if profile != nil {
		command.queuedAt = time.Now()
	}
	stopAdmission := handoff.CaptureSpan(profile, handoff.CaptureAdmission)
	select {
	case current.commands <- command:
	case <-current.done:
		stopAdmission()
		return current.stoppedError()
	case <-timer.C:
		stopAdmission()
		err := fmt.Errorf("source wheel capture command admission timed out: %w", ErrSourceCaptureCommandTimeout)
		current.fail(err)
		return err
	}
	stopAdmission()
	stopPost := handoff.CaptureSpan(profile, handoff.CapturePost)
	posted, _, postErr := procPostMessageWTouchpad.Call(current.window, uintptr(message), 0, 0)
	stopPost()
	if posted == 0 {
		select {
		case <-current.commands:
		default:
		}
		err := touchpadWin32Error("PostMessageW(source wheel capture)", postErr)
		current.fail(err)
		return err
	}
	stopResult := handoff.CaptureSpan(profile, handoff.CaptureResultWait)
	defer stopResult()
	select {
	case err := <-command.result:
		return err
	case <-current.done:
		return current.stoppedError()
	case <-timer.C:
		err := fmt.Errorf("source wheel capture command result timed out: %w", ErrSourceCaptureCommandTimeout)
		current.fail(err)
		return err
	}
}

func (current *sourceWheelCaptureRun) stoppedError() error {
	if err := current.err(); err != nil {
		return fmt.Errorf("source wheel capture window stopped: %w", err)
	}
	return errors.New("source wheel capture window stopped unexpectedly")
}

func (capture *sourceWheelCapture) failCurrentRun(cause error) {
	capture.runMu.RLock()
	current := capture.run
	capture.runMu.RUnlock()
	if current == nil {
		return
	}
	current.fail(cause)
}

func (capture *sourceWheelCapture) runWindow(
	ctx context.Context,
	session *SourceSession,
	ready func(),
) error {
	if capture == nil || session == nil {
		return errors.New("source wheel capture and session are required")
	}
	registerProc, available := resolveNamedRegisterTouchpadCapableWindow()
	if !available {
		return errors.Join(lab.ErrUnsupported, errors.New("RegisterTouchpadCapableWindow is unavailable"))
	}
	current := &sourceWheelCaptureRun{
		shutdownEvidence: sourceShutdownFrom(ctx),
		commands:         make(chan sourceWheelCaptureCommand, 1),
		done:             make(chan struct{}),
		registerProc:     registerProc,
		session:          session,
		observer:         capture.observer,
	}
	if err := capture.attach(current); err != nil {
		return err
	}
	defer capture.detach(current)
	defer close(current.done)
	return current.run(ctx, ready)
}

func resolveNamedRegisterTouchpadCapableWindow() (uintptr, bool) {
	named := user32.NewProc("RegisterTouchpadCapableWindow")
	if named.Find() != nil {
		return 0, false
	}
	return named.Addr(), true
}

func (capture *sourceWheelCapture) attach(current *sourceWheelCaptureRun) error {
	capture.runMu.Lock()
	defer capture.runMu.Unlock()
	if capture.run != nil {
		return errors.New("source wheel capture is already running")
	}
	capture.run = current
	return nil
}

func (capture *sourceWheelCapture) detach(current *sourceWheelCaptureRun) {
	capture.runMu.Lock()
	if capture.run == current {
		capture.run = nil
	}
	capture.runMu.Unlock()
}

func (current *sourceWheelCaptureRun) run(ctx context.Context, ready func()) (resultErr error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	module, _, callErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		return touchpadWin32Error("GetModuleHandleW", callErr)
	}
	className, err := syscall.UTF16PtrFromString(fmt.Sprintf("KrasisSourceWheelCapture-%d-%d", os.Getpid(), sourceWheelCaptureClassSequence.Add(1)))
	if err != nil {
		return fmt.Errorf("encode source wheel capture class name: %w", err)
	}
	windowTitle, err := syscall.UTF16PtrFromString("Krasis remote input capture")
	if err != nil {
		return fmt.Errorf("encode source wheel capture window title: %w", err)
	}
	windowDestroyed := false
	wndProc := sourceWindowCallback(func(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
		switch message {
		case 0x20: // WM_SETCURSOR
			if current.hideActiveCursor() {
				return 1
			}
		case wmSourceWheelCaptureActivate, wmSourceWheelCaptureDeactivate:
			return current.handleCommand(message)
		case wmSourceWheelCaptureStop:
			procPostQuitMessageTouchpad.Call(0)
			return 0
		case wmMouseWheel, wmMouseHWheel:
			if current.active.Load() {
				current.routeWheel(message, wParam)
				return 0
			}
		case wmActivate:
			if uint16(wParam) == waInactive && current.loseForeground() {
				procPostQuitMessageTouchpad.Call(0)
				return 0
			}
		case wmPointerDown, wmPointerUpdate, wmPointerUp, wmPointerEnter, wmPointerLeave,
			wmPointerCaptureChanged, wmPointerWheel, wmPointerHorizontalWheel:
			// DefWindowProc performs the system touchpad gesture conversion. The
			// source owner consumes only the resulting legacy wheel messages.
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
		WindowProc: sourceCaptureCallback,
		Instance:   module,
		Cursor:     cursor,
		Background: colorWindowBrush,
		ClassName:  className,
	}
	atom, _, callErr := procRegisterClassWTouchpad.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return touchpadWin32Error("RegisterClassW(source wheel capture)", callErr)
	}
	defer func() {
		if unregistered, _, unregisterErr := procUnregisterClassWTouchpad.Call(uintptr(unsafe.Pointer(className)), module); unregistered == 0 {
			resultErr = errors.Join(resultErr, touchpadWin32Error("UnregisterClassW(source wheel capture)", unregisterErr))
		}
		runtime.KeepAlive(wndProc)
	}()
	window, _, callErr := procCreateWindowExW.Call(
		wsExTopmost|wsExToolWindow|wsExLayeredSourceCapture,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		wsPopup,
		0, 0, sourceWheelCaptureWidth, sourceWheelCaptureHeight,
		0, 0, module, 0,
	)
	if window == 0 {
		return touchpadWin32Error("CreateWindowExW(source wheel capture)", callErr)
	}
	current.window = window
	sourceWindowOwners.Store(window, wndProc)
	defer sourceWindowOwners.Delete(window)
	defer func() {
		resultErr = errors.Join(resultErr, current.deactivateWindow())
		if !windowDestroyed {
			if destroyed, _, destroyErr := procDestroyWindow.Call(window); destroyed == 0 {
				resultErr = errors.Join(resultErr, touchpadWin32Error("DestroyWindow(source wheel capture)", destroyErr))
			}
		}
	}()

	// Configure before the first ShowWindow: an opaque frame must never flash.
	if err := configureSourceWheelCaptureAppearance(window); err != nil {
		return err
	}

	stopWatcher := make(chan struct{})
	stopWatcherDone := make(chan struct{})
	go func() {
		defer close(stopWatcherDone)
		select {
		case <-ctx.Done():
			current.fail(ctx.Err())
		case <-stopWatcher:
		}
	}()
	defer func() {
		close(stopWatcher)
		<-stopWatcherDone
	}()
	if ready != nil {
		ready()
	}
	loopErr := touchpadSurfaceMessageLoop()
	if loopErr != nil {
		return errors.Join(loopErr, current.err())
	}
	if terminalErr := current.err(); terminalErr != nil {
		return terminalErr
	}
	return ctx.Err()
}

func configureSourceWheelCaptureAppearance(window uintptr) error {
	// Alpha zero or WS_EX_TRANSPARENT would pass hit testing through to other
	// applications. Keep a nonzero alpha so touchpad wheel ownership is retained.
	configured, _, callErr := callSetLayeredSourceCapture(window, 0, sourceWheelCaptureAlpha, sourceWheelCaptureAlphaFlag)
	if configured == 0 {
		return fmt.Errorf("%w: %v", ErrSourceCaptureActivationSetup, touchpadWin32Error("SetLayeredWindowAttributes(source wheel capture)", callErr))
	}
	return nil
}

func (current *sourceWheelCaptureRun) handleCommand(message uint32) uintptr {
	var command sourceWheelCaptureCommand
	select {
	case command = <-current.commands:
	default:
		current.setError(errors.New("source wheel capture command message had no request"))
		procPostQuitMessageTouchpad.Call(0)
		return 0
	}
	if !command.queuedAt.IsZero() {
		handoff.RecordCapture(command.profile, handoff.CaptureQueueToHandle, time.Since(command.queuedAt))
	}
	stopHandler := handoff.CaptureSpan(command.profile, handoff.CaptureHandlerTotal)
	if command.message != message {
		err := errors.New("source wheel capture command order mismatch")
		current.setError(err)
		stopHandler()
		command.result <- err
		procPostQuitMessageTouchpad.Call(0)
		return 0
	}
	var err error
	if terminalErr := current.err(); terminalErr != nil {
		stopHandler()
		command.result <- terminalErr
		return 0
	}
	switch message {
	case wmSourceWheelCaptureActivate:
		err = current.activateWindowProfile(command.profile)
	case wmSourceWheelCaptureDeactivate:
		err = current.deactivateWindowProfile(command.profile)
	}
	stopHandler()
	command.result <- err
	_, rollbackConfirmed := err.(*sourceCaptureActivationRejected)
	if err != nil && !rollbackConfirmed {
		current.setError(err)
		procPostQuitMessageTouchpad.Call(0)
	}
	return 0
}

func (current *sourceWheelCaptureRun) activateWindow() (activationErr error) {
	return current.activateWindowProfile(nil)
}

func (current *sourceWheelCaptureRun) activateWindowProfile(profile context.Context) (activationErr error) {
	if err := current.err(); err != nil {
		return err
	}
	if current.active.Load() {
		return errors.New("source wheel capture is already active")
	}
	var missingForegroundStage error
	defer func() {
		if activationErr != nil {
			if errors.Is(activationErr, ErrSourceForegroundUnavailable) {
				activationErr = errors.Join(activationErr, missingForegroundStage)
			}
			stopRollback := handoff.CaptureSpan(profile, handoff.CaptureRollback)
			activationErr = current.rollbackActivationProfile(profile, activationErr)
			stopRollback()
		}
	}()
	stopPosition := handoff.CaptureSpan(profile, handoff.CapturePosition)
	position, err := cursorPosition()
	if err != nil {
		stopPosition()
		return fmt.Errorf("%w: position source wheel capture: %v", ErrSourceCaptureActivationSetup, err)
	}
	current.savedFocus = sourceWheelCaptureGetForeground()
	handoff.RecordFocusState(profile, handoff.FocusBeforeShow, sourceFocusState(current.savedFocus, current.window, current.savedFocus))
	if current.savedFocus == 0 {
		missingForegroundStage = ErrSourceForegroundMissingBeforeShow
	}
	if err := current.err(); err != nil {
		stopPosition()
		return err
	}
	x, y := sourceWheelCapturePosition(position)
	if positionErr := sourceWheelCaptureSetPosition(current.window, x, y); positionErr != nil {
		stopPosition()
		return fmt.Errorf("%w: %v", ErrSourceCaptureActivationSetup, positionErr)
	}
	stopPosition()
	if err := current.err(); err != nil {
		return err
	}
	current.registered = true // Also attempt unregister if registration reports failure.
	stopRegister := handoff.CaptureSpan(profile, handoff.CaptureTouchpadRegister)
	registerErr := sourceWheelCaptureRegisterWindow(current.registerProc, current.window, true)
	stopRegister()
	if registerErr != nil {
		sourceWheelCaptureShowWindow(current.window, swHide)
		return fmt.Errorf("%w: %v", ErrSourceCaptureActivationSetup, touchpadWin32Error("RegisterTouchpadCapableWindow(source TRUE)", registerErr))
	}
	if err := current.err(); err != nil {
		return err
	}
	// Showing must not separately activate the popup and discard the foreground
	// owner needed by the verified SetForegroundWindow/thread-attach path below.
	current.inheritFullscreenPresentation()
	stopShow := handoff.CaptureSpan(profile, handoff.CaptureShow)
	sourceWheelCaptureShowWindow(current.window, swShowWithoutActivation)
	stopShow()
	if err := current.err(); err != nil {
		return err
	}
	stopUpdate := handoff.CaptureSpan(profile, handoff.CaptureUpdate)
	procUpdateWindowTouchpad.Call(current.window)
	stopUpdate()
	if err := current.err(); err != nil {
		return err
	}
	if sourceWheelCaptureGetForeground() == 0 && missingForegroundStage == nil {
		missingForegroundStage = ErrSourceForegroundMissingAfterShow
	}
	if err := current.err(); err != nil {
		return err
	}
	stopForeground := handoff.CaptureSpan(profile, handoff.CaptureForeground)
	foregroundErr := sourceWheelCaptureSetForeground(current.window)
	if err := current.err(); err != nil {
		stopForeground()
		return err
	}
	if foregroundErr == nil && sourceWheelCaptureGetForeground() != current.window {
		// Complete a pending request on our owner queue before joining the
		// previous application's queue. A non-NULL previous foreground is
		// also possible while activation is pending, not only a NULL owner.
		if err := current.err(); err != nil {
			stopForeground()
			return err
		}
		sourceForegroundTransitionDispatch(current.window)
		if err := current.err(); err != nil {
			stopForeground()
			return err
		}
	}
	if sourceWheelCaptureGetForeground() == 0 && missingForegroundStage == nil {
		missingForegroundStage = ErrSourceForegroundMissingAfterRequest
	}
	if sourceWheelCaptureGetForeground() == current.window {
		foregroundErr = nil
	}
	verificationLost := foregroundErr == nil && sourceWheelCaptureGetForeground() != current.window
	stopForeground()
	if foregroundErr != nil || verificationLost {
		stopFallback := handoff.CaptureSpan(profile, handoff.CaptureFallback)
		fallbackResult, fallbackErr := current.activateForegroundFallback()
		stopFallback()
		if current.observer != nil {
			current.observer.ObserveWheelForegroundFallback(fallbackResult)
		}
		if fallbackErr == nil {
			foregroundErr = nil
			verificationLost = false
		} else {
			if foregroundErr != nil {
				return errors.Join(
					fmt.Errorf("%w: %v", ErrSourceCaptureActivationAPIRejected, foregroundErr),
					fallbackErr,
				)
			}
			return errors.Join(ErrSourceCaptureActivationVerificationLost, fallbackErr)
		}
	}
	if foregroundErr != nil {
		return fmt.Errorf("%w: %v", ErrSourceCaptureActivationAPIRejected, foregroundErr)
	}
	if verificationLost || sourceWheelCaptureGetForeground() != current.window {
		return ErrSourceCaptureActivationVerificationLost
	}
	stopCommit := handoff.CaptureSpan(profile, handoff.CaptureActiveCommit)
	defer stopCommit()
	current.terminalMu.Lock()
	if current.terminalErr != nil {
		err := current.terminalErr
		current.terminalMu.Unlock()
		return err
	}
	current.active.Store(true)
	current.terminalMu.Unlock()
	current.hideActiveCursor()
	if current.observer != nil {
		current.observer.ObserveWheelCaptureActivated()
	}
	return nil
}

// Run only on the capture window's owner thread. Inactive windows retain the
// normal class cursor; no global cursor resource or ShowCursor count is changed.
func (current *sourceWheelCaptureRun) hideActiveCursor() bool {
	if !current.active.Load() {
		return false
	}
	callSetSourceCaptureCursor(0)
	return true
}

func (current *sourceWheelCaptureRun) activateForegroundFallback() (SourceForegroundFallbackResult, error) {
	foreground, foregroundErr := current.foregroundAfterTransition()
	if foregroundErr != nil {
		return SourceForegroundFallbackAttachFailed, errors.Join(ErrSourceCaptureThreadAttachFailed, foregroundErr)
	}
	if foreground == current.window && foreground != 0 {
		return SourceForegroundFallbackSucceeded, nil
	}
	result, err := current.setForegroundWithThreadAttach(current.window, foreground)
	switch result {
	case foregroundThreadFallbackSucceeded:
		return SourceForegroundFallbackSucceeded, nil
	case foregroundThreadFallbackAttachFailed:
		return SourceForegroundFallbackAttachFailed, errors.Join(ErrSourceCaptureThreadAttachFailed, err)
	case foregroundThreadFallbackDetachFailed:
		return SourceForegroundFallbackDetachFailed, errors.Join(ErrSourceCaptureThreadDetachFailed, err)
	case foregroundThreadFallbackAPIRejected:
		return SourceForegroundFallbackAPIRejected, errors.Join(ErrSourceCaptureActivationAPIRejected, err)
	case foregroundThreadFallbackVerificationLost:
		return SourceForegroundFallbackVerificationLost, errors.Join(ErrSourceCaptureActivationVerificationLost, err)
	default:
		return SourceForegroundFallbackAttachFailed, errors.Join(
			ErrSourceCaptureThreadAttachFailed,
			errors.New("source capture foreground fallback returned an invalid result"),
		)
	}
}

func (current *sourceWheelCaptureRun) setForegroundWithThreadAttach(targetWindow, peerWindow uintptr) (foregroundThreadFallbackResult, error) {
	result, _, _, err := current.setForegroundWithThreadAttachDetailed(targetWindow, peerWindow, nil)
	return result, err
}

func (current *sourceWheelCaptureRun) deactivateWindow() error {
	return current.deactivateWindowProfile(nil)
}

func (current *sourceWheelCaptureRun) deactivateWindowProfile(profile context.Context) error {
	wasActive := current.active.Swap(false)
	wasRegistered := current.registered
	// Retry only the exact native pair we attached, never a newly discovered owner.
	detachErr := current.detachForegroundThreads()
	if !wasActive && !wasRegistered {
		current.releaseFullscreenPresentation()
		return detachErr
	}
	if current.observer != nil && wasActive {
		current.observer.ObserveWheelCaptureDeactivated()
	}
	cleanupErr := detachErr
	if wasRegistered {
		if unregisterErr := sourceWheelCaptureRegisterWindow(current.registerProc, current.window, false); unregisterErr != nil {
			cleanupErr = errors.Join(cleanupErr, ErrSourceCaptureUnregisterFailed, touchpadWin32Error("RegisterTouchpadCapableWindow(source FALSE)", unregisterErr))
		} else {
			current.registered = false
		}
	}
	// Restore while the capture window is still foreground. Hiding it first lets
	// Windows choose another foreground owner, which makes restoreFocus classify
	// every ordinary return as skipped and loses the saved application focus.
	current.observeFocus(profile, handoff.FocusBeforeRestore)
	stopRestore := handoff.FocusSpan(profile, 0)
	restoreResult, restoreErr := current.restoreFocusProfile(profile)
	stopRestore()
	handoff.RecordFocusResult(profile, 0, uint8(restoreResult))
	current.observeFocus(profile, handoff.FocusAfterRestore)
	stopHide := handoff.FocusSpan(profile, 1)
	hideErr := current.hideAfterFocusRestore()
	current.releaseFullscreenPresentation()
	stopHide()
	if hideErr != nil {
		cleanupErr = errors.Join(cleanupErr, hideErr, ErrSourceCaptureRollbackFailed)
	}
	if current.observer != nil && wasActive {
		current.observer.ObserveWheelFocusRestore(restoreResult)
	}
	if wasActive && cleanupErr == nil {
		// The first restore's failure remains recorded. Only a fresh NULL owner
		// after hide permits this separate, one-shot saved-window request.
		current.restoreAfterHideProfile(profile, restoreResult, restoreErr)
	}
	savedFocus := current.savedFocus
	current.savedFocus = 0
	// Partial activation can own registration and foreground before active=true.
	// It needs the same post-hide release proof as a fully active capture.
	postHideResult, postHideErr := current.verifyPostHideFocusReleaseProfile(profile, savedFocus)
	if restoreErr == nil {
		return errors.Join(cleanupErr, postHideErr)
	}
	if current.observer != nil {
		current.observer.ObserveWheelPostHideFocus(postHideResult)
	}
	if errors.Is(restoreErr, ErrSourceFocusRestoreThreadDetachFailed) || postHideErr != nil {
		return errors.Join(cleanupErr, restoreErr, postHideErr)
	}
	return cleanupErr
}

func (current *sourceWheelCaptureRun) verifyPostHideFocusRelease() (SourcePostHideFocusResult, error) {
	return current.verifyPostHideFocusReleaseProfile(nil, 0)
}

func (current *sourceWheelCaptureRun) verifyPostHideFocusReleaseProfile(profile context.Context, savedFocus uintptr) (result SourcePostHideFocusResult, err error) {
	foreground := sourceWheelCaptureGetForeground()
	handoff.RecordFocusState(profile, handoff.FocusAfterHide, sourceFocusState(foreground, current.window, savedFocus))
	defer func() { handoff.RecordFocusResult(profile, 1, uint8(result)) }()
	switch foreground {
	case current.window:
		return SourcePostHideFocusCaptureRetained, ErrSourceFocusReleaseCaptureRetained
	case 0:
		return SourcePostHideFocusUnavailable, nil
	default:
		return SourcePostHideFocusRecovered, nil
	}
}

func sourceFocusState(foreground, capture, saved uintptr) uint8 {
	switch {
	case foreground == 0:
		return 1
	case foreground == capture:
		return 2
	case foreground == saved:
		return 3
	default:
		return 4
	}
}

func (current *sourceWheelCaptureRun) observeFocus(profile context.Context, stage handoff.FocusStage) {
	if handoff.FocusEnabled(profile) {
		handoff.RecordFocusState(profile, stage, sourceFocusState(sourceWheelCaptureGetForeground(), current.window, current.savedFocus))
	}
}

func (current *sourceWheelCaptureRun) restoreFocus() (SourceFocusRestoreResult, error) {
	return current.restoreFocusProfile(nil)
}

func (current *sourceWheelCaptureRun) restoreFocusProfile(profile context.Context) (SourceFocusRestoreResult, error) {
	if current.savedFocus == 0 {
		return SourceFocusRestoreSkipped, nil
	}
	if sourceWheelCaptureGetForeground() != current.window {
		return SourceFocusRestoreSkipped, nil
	}
	if !sourceWheelCaptureIsWindow(current.savedFocus) {
		return SourceFocusRestoreSkipped, nil
	}
	restoreErr := sourceWheelCaptureSetForeground(current.savedFocus)
	verificationLost := restoreErr == nil && sourceWheelCaptureGetForeground() != current.savedFocus
	if verificationLost {
		// Cross-queue SetForegroundWindow can return before activation finishes.
		// WM_NULL waits at most the existing 100ms probe deadline, without
		// joining the application's input queue or synthesizing a click.
		if sourceWheelCaptureProbeForeground(current.savedFocus) == nil {
			verificationLost = sourceWheelCaptureGetForeground() != current.savedFocus
		}
	}
	if restoreErr != nil || verificationLost {
		fallbackResult, fallbackErr := current.restoreFocusFallbackProfile(profile)
		if current.observer != nil {
			current.observer.ObserveWheelFocusRestoreFallback(fallbackResult)
		}
		if fallbackErr == nil {
			return SourceFocusRestoreSucceeded, nil
		}
		if restoreErr != nil {
			return SourceFocusRestoreAPIRejected, errors.Join(
				fmt.Errorf("%w: %v", ErrSourceFocusRestoreAPIRejected, restoreErr),
				fallbackErr,
			)
		}
		return SourceFocusRestoreVerificationLost, errors.Join(ErrSourceFocusRestoreVerificationLost, fallbackErr)
	}
	return SourceFocusRestoreSucceeded, nil
}

func (current *sourceWheelCaptureRun) restoreFocusFallback() (SourceFocusRestoreFallbackResult, error) {
	return current.restoreFocusFallbackProfile(nil)
}

func (current *sourceWheelCaptureRun) restoreFocusFallbackProfile(profile context.Context) (mapped SourceFocusRestoreFallbackResult, mappedErr error) {
	result, stage, rejectedForeground, err := current.setForegroundWithThreadAttachDetailed(current.savedFocus, current.savedFocus, profile)
	defer func() {
		handoff.RecordFocusFallbackObserved(profile, uint8(mapped), stage, rejectedForeground)
	}()
	switch result {
	case foregroundThreadFallbackSucceeded:
		return SourceFocusRestoreFallbackSucceeded, nil
	case foregroundThreadFallbackAttachFailed:
		return SourceFocusRestoreFallbackAttachFailed, errors.Join(ErrSourceFocusRestoreThreadAttachFailed, err)
	case foregroundThreadFallbackDetachFailed:
		return SourceFocusRestoreFallbackDetachFailed, errors.Join(ErrSourceFocusRestoreThreadDetachFailed, err)
	case foregroundThreadFallbackAPIRejected:
		return SourceFocusRestoreFallbackAPIRejected, errors.Join(ErrSourceFocusRestoreAPIRejected, err)
	case foregroundThreadFallbackVerificationLost:
		return SourceFocusRestoreFallbackVerificationLost, errors.Join(ErrSourceFocusRestoreVerificationLost, err)
	default:
		return SourceFocusRestoreFallbackAttachFailed, errors.Join(
			ErrSourceFocusRestoreThreadAttachFailed,
			errors.New("source focus restore fallback returned an invalid result"),
		)
	}
}

func (current *sourceWheelCaptureRun) routeWheel(message uint32, wParam uintptr) {
	action, emitted, err := mapWindowMouseWheel(message, wParam)
	if err != nil {
		current.session.Abort(err)
		return
	}
	if !emitted {
		return
	}
	if current.observer != nil {
		current.observer.ObserveWheelWindow()
		current.observer.ObserveWheelMapped()
	}
	if !current.session.Active() {
		// Source forwarding is stopped before native capture cleanup. A queued
		// wheel message during that interval belongs to the retired owner.
		return
	}
	current.session.Offer(action)
}

func mapWindowMouseWheel(message uint32, wParam uintptr) (control.InputAction, bool, error) {
	delta := int32(int16(uint16(wParam >> 16)))
	if delta == 0 {
		return control.InputAction{}, false, nil
	}
	var action control.InputAction
	switch message {
	case wmMouseWheel:
		action = control.InputAction{Kind: control.InputWheel, DeltaY: delta}
	case wmMouseHWheel:
		action = control.InputAction{Kind: control.InputWheel, DeltaX: delta}
	default:
		return control.InputAction{}, false, nil
	}
	if err := action.Validate(); err != nil {
		return control.InputAction{}, false, fmt.Errorf("map source window wheel: %w", err)
	}
	return action, true, nil
}

func sourceWheelCapturePosition(position point) (int32, int32) {
	left := systemMetric(smXVIRTUALSCREEN)
	top := systemMetric(smYVIRTUALSCREEN)
	right := left + systemMetric(smCXVIRTUALSCREEN)
	bottom := top + systemMetric(smCYVIRTUALSCREEN)
	x := position.X - sourceWheelCaptureWidth/2
	y := position.Y - sourceWheelCaptureHeight/2
	if x < left {
		x = left
	}
	if x+sourceWheelCaptureWidth > right {
		x = right - sourceWheelCaptureWidth
	}
	if y < top {
		y = top
	}
	if y+sourceWheelCaptureHeight > bottom {
		y = bottom - sourceWheelCaptureHeight
	}
	return x, y
}

func (current *sourceWheelCaptureRun) setError(err error) {
	if err == nil {
		return
	}
	current.terminalMu.Lock()
	current.terminalErr = errors.Join(current.terminalErr, err)
	current.active.Store(false)
	current.terminalMu.Unlock()
	if current.session != nil {
		current.session.Abort(err)
	}
}

func (current *sourceWheelCaptureRun) loseForeground() bool {
	if !current.active.Load() {
		return false
	}
	if current.session != nil && current.session.RequestLocalReturn() {
		current.active.Store(false)
		return false // Keep the owner loop alive for cleanup and the next crossing.
	}
	current.setError(ErrSourceCaptureLostForeground)
	return true
}

func (current *sourceWheelCaptureRun) fail(err error) {
	current.setError(err)
	if current.window != 0 {
		postErr := postSourceShutdown(current.shutdownEvidence, func() error {
			posted, _, err := procPostMessageWTouchpad.Call(current.window, wmSourceWheelCaptureStop, 0, 0)
			if posted == 0 {
				return touchpadWin32Error("PostMessageW(source stop)", err)
			}
			return nil
		})
		current.setError(postErr)
	}
}

func (current *sourceWheelCaptureRun) err() error {
	current.terminalMu.Lock()
	defer current.terminalMu.Unlock()
	return current.terminalErr
}
