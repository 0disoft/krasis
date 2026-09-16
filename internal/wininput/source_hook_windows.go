//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/control"
)

const wmSourceReady = 0x8002

type SourceHook struct {
	observer     SourcePipelineObserver
	capture      *sourceWheelCapture
	returnCursor returnCursorShield
}

type sourceInputThreadResult struct {
	component string
	err       error
}

type serializedBoundaryObserver struct {
	mu   sync.Mutex
	next BoundaryObserver
}

func (observer *serializedBoundaryObserver) Observe(action control.InputAction, pointerX int32) error {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	return observer.next.Observe(action, pointerX)
}

func NewSourceHook() *SourceHook {
	return NewSourceHookObserved(nil)
}

func NewSourceHookObserved(observer SourcePipelineObserver) *SourceHook {
	return &SourceHook{observer: observer, capture: newSourceWheelCapture(observer)}
}

func (hook *SourceHook) SourceCapture() SourceActivation {
	if hook == nil {
		return nil
	}
	return hook.capture
}

func (hook *SourceHook) Run(ctx context.Context, session *SourceSession, ready func()) error {
	return hook.RunWithBoundary(ctx, session, nil, ready)
}

func (hook *SourceHook) RunWithBoundary(
	ctx context.Context,
	session *SourceSession,
	boundary BoundaryObserver,
	ready func(),
) error {
	if hook == nil || hook.capture == nil || session == nil {
		return errors.New("source hook and session are required")
	}
	if ctx == nil {
		return errors.New("source hook context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	releaseHookSession, err := acquireHookSession(ctx)
	if err != nil {
		return err
	}
	releaseAllowed := true
	defer func() {
		if releaseAllowed {
			releaseHookSession()
		}
	}()
	initialPosition, err := cursorPosition()
	if err != nil {
		return fmt.Errorf("snapshot source pointer: %w", err)
	}
	pointerState := newSourcePointerState(initialPosition)
	session.physicalMu.Lock()
	session.physicalCheck = sourceAnyInputPressed
	session.physicalMu.Unlock()
	if boundary != nil {
		boundary = &serializedBoundaryObserver{next: boundary}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	readySignals := make(chan struct{}, 3)
	results := make(chan sourceInputThreadResult, 3)
	startSourceInputThread(results, "low-level-hook", func() error {
		return runSourceLowLevelHookThread(runCtx, session, boundary, hook.observer, hook.capture, pointerState, func() {
			readySignals <- struct{}{}
		}, &hook.returnCursor)
	})
	startSourceInputThread(results, "raw-mouse", func() error {
		return runSourceRawMouseThread(runCtx, session, boundary, pointerState, func() {
			readySignals <- struct{}{}
		})
	})
	startSourceInputThread(results, "wheel-capture", func() error {
		return hook.capture.runWindow(runCtx, session, func() {
			readySignals <- struct{}{}
		})
	})

	readyCount := 0
	for readyCount < 3 {
		select {
		case <-readySignals:
			readyCount++
		case first := <-results:
			resultErr, joined := collectSourceInputThreads(ctx, cancel, results, []sourceInputThreadResult{first}, 3, SourceCleanupTimeout)
			releaseAllowed = joined
			return resultErr
		case <-ctx.Done():
			resultErr, joined := collectSourceInputThreads(ctx, cancel, results, nil, 3, SourceCleanupTimeout)
			releaseAllowed = joined
			return resultErr
		}
	}
	if ready != nil {
		ready()
	}
	select {
	case first := <-results:
		resultErr, joined := collectSourceInputThreads(ctx, cancel, results, []sourceInputThreadResult{first}, 3, SourceCleanupTimeout)
		releaseAllowed = joined
		return resultErr
	case <-ctx.Done():
		resultErr, joined := collectSourceInputThreads(ctx, cancel, results, nil, 3, SourceCleanupTimeout)
		releaseAllowed = joined
		return resultErr
	}
}

func startSourceInputThread(results chan<- sourceInputThreadResult, component string, run func() error) {
	go func() {
		runtime.LockOSThread()
		results <- sourceInputThreadResult{component: component, err: run()}
	}()
}

var errSourceInputJoinTimeout = errors.New("source input threads did not stop before the cleanup deadline")

func collectSourceInputThreads(
	parent context.Context,
	cancel context.CancelFunc,
	results <-chan sourceInputThreadResult,
	collected []sourceInputThreadResult,
	expected int,
	timeout time.Duration,
) (error, bool) {
	cancel()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for len(collected) < expected {
		select {
		case result := <-results:
			collected = append(collected, result)
		case <-timer.C:
			collected = append(collected, sourceInputThreadResult{component: "thread-join", err: errSourceInputJoinTimeout})
			return sourceInputThreadsError(parent, collected...), false
		}
	}
	return sourceInputThreadsError(parent, collected...), true
}

func sourceInputThreadsError(parent context.Context, results ...sourceInputThreadResult) error {
	parentErr := parent.Err()
	joined := make([]error, 0, len(results))
	for _, result := range results {
		if result.err != nil && parentErr != nil && cancellationOnly(result.err) {
			continue
		}
		if result.err == nil {
			if parentErr != nil {
				continue
			}
			joined = append(joined, fmt.Errorf("source %s stopped unexpectedly", result.component))
			continue
		}
		joined = append(joined, fmt.Errorf("source %s: %w", result.component, result.err))
	}
	if len(joined) > 0 {
		joined = append([]error{parentErr}, joined...)
		return errors.Join(joined...)
	}
	if parentErr != nil {
		return parentErr
	}
	return context.Canceled
}

func cancellationOnly(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !cancellationOnly(cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return cancellationOnly(wrapped.Unwrap())
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func runSourceLowLevelHookThread(
	ctx context.Context,
	session *SourceSession,
	boundary BoundaryObserver,
	observer SourcePipelineObserver,
	wheelCapture SourceActivation,
	pointerState *sourcePointerState,
	ready func(),
	shields ...*returnCursorShield,
) error {
	var shield *returnCursorShield
	if len(shields) != 0 {
		shield = shields[0]
		shield.open()
		defer shield.close()
	}
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)
	mapper := &sourceKeyboardMapper{}
	keyboardCallback := func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			shield.beforeInput()
			if routeMappedPhysicalAction(session, boundary, func() (control.InputAction, bool, int32, error) {
				data := *(*keyboardHookData)(unsafe.Pointer(lParam))
				action, emitted, err := mapper.keyboard(uint32(wParam), data)
				return action, emitted, 0, err
			}) {
				return 1
			}
		}
		return callNextHook(code, wParam, lParam)
	}
	mouseCallback := func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			shield.beforeInput()
			data := *(*mouseHookData)(unsafe.Pointer(lParam))
			message := uint32(wParam)
			if action, button := mapSourceHookButton(message, data); button {
				if routePhysicalAction(session, boundary, action, data.Point.X) {
					return 1
				}
				return callNextHook(code, wParam, lParam)
			}
			if message == wmMouseMove {
				pointerState.observe(data.Point)
			}
			if message == wmMouseWheel || message == wmMouseHWheel {
				if ownedByWindow, suppress := sourceWindowOwnsHookWheel(wheelCapture, data); ownedByWindow {
					if suppress {
						return 1
					}
					observeHookMouseWheel(observer, data)
					return callNextHook(code, wParam, lParam)
				}
				if routeMappedPhysicalAction(session, boundary, func() (control.InputAction, bool, int32, error) {
					action, emitted, err := mapObservedHookMouseWheel(observer, message, data)
					return action, emitted, data.Point.X, err
				}) {
					return 1
				}
			}
			activePhysical := shouldSuppressPhysicalMouseInput(session, data)
			observeActivePhysicalHookMouse(observer, message, activePhysical)
			if activePhysical {
				return 1
			}
		}
		return callNextHook(code, wParam, lParam)
	}
	owner := &sourceHookCallbacks{keyboard: keyboardCallback, mouse: mouseCallback}
	if _, loaded := sourceHookOwners.LoadOrStore(threadID, owner); loaded {
		return errors.New("source hook thread already has an owner")
	}
	defer sourceHookOwners.CompareAndDelete(threadID, owner)

	module, _, callErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		return fmt.Errorf("GetModuleHandleW failed: %w", callErr)
	}
	keyboardHook, _, callErr := procSetWindowsHookExW.Call(whKeyboardLL, sourceKeyboardCallback, module, 0)
	if keyboardHook == 0 {
		return fmt.Errorf("SetWindowsHookExW(WH_KEYBOARD_LL) failed: %w", callErr)
	}
	mouseHook, _, callErr := procSetWindowsHookExW.Call(whMouseLL, sourceMouseCallback, module, 0)
	if mouseHook == 0 {
		procUnhookWindowsHookEx.Call(keyboardHook)
		return fmt.Errorf("SetWindowsHookExW(WH_MOUSE_LL) failed: %w", callErr)
	}
	cleanupHooks := func() error {
		var cleanupErr error
		if mouseHook != 0 {
			result, _, unhookErr := procUnhookWindowsHookEx.Call(mouseHook)
			mouseHook = 0
			if result == 0 {
				cleanupErr = fmt.Errorf("UnhookWindowsHookEx(WH_MOUSE_LL) failed: %w", unhookErr)
			}
		}
		if keyboardHook != 0 {
			result, _, unhookErr := procUnhookWindowsHookEx.Call(keyboardHook)
			keyboardHook = 0
			if result == 0 && cleanupErr == nil {
				cleanupErr = fmt.Errorf("UnhookWindowsHookEx(WH_KEYBOARD_LL) failed: %w", unhookErr)
			}
		}
		return cleanupErr
	}
	defer cleanupHooks()
	if err := postSourceReady(threadID); err != nil {
		return err
	}
	loopErr := runSourceHookMessageLoop(ctx, threadID, ready)
	unhookErr := cleanupHooks()
	if loopErr != nil {
		return errors.Join(loopErr, unhookErr)
	}
	if unhookErr != nil {
		return unhookErr
	}
	return ctx.Err()
}

func runSourceRawMouseThread(
	ctx context.Context,
	session *SourceSession,
	boundary BoundaryObserver,
	pointerState *sourcePointerState,
	ready func(),
) error {
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)
	window, err := createRawInputMessageWindow("KrasisSourceRawMouse")
	if err != nil {
		return err
	}
	defer procDestroyWindow.Call(window)
	if err := registerRawMouseInput(window); err != nil {
		return err
	}
	rawRegistered := true
	cleanupRawInput := func() error {
		if !rawRegistered {
			return nil
		}
		rawRegistered = false
		return unregisterRawMouseInput()
	}
	defer cleanupRawInput()
	rawReader, err := newRawMouseReader()
	if err != nil {
		return err
	}
	if err := postSourceReady(threadID); err != nil {
		return err
	}
	visitRawMouse := func(header rawInputHeader, mouse rawMouse) error {
		return routeRawMouse(session, boundary, pointerState, header, mouse)
	}
	loopErr := runSourceRawMessageLoop(ctx, threadID, ready, rawReader, visitRawMouse)
	rawInputErr := cleanupRawInput()
	if loopErr != nil {
		return errors.Join(loopErr, rawInputErr)
	}
	if rawInputErr != nil {
		return rawInputErr
	}
	return ctx.Err()
}

func postSourceReady(threadID uintptr) error {
	result, _, postErr := procPostThreadMessageW.Call(threadID, wmSourceReady, 0, 0)
	if result == 0 {
		return fmt.Errorf("PostThreadMessageW(source ready) failed: %w", postErr)
	}
	return nil
}

func runSourceHookMessageLoop(ctx context.Context, threadID uintptr, ready func()) error {
	return runSourceThreadMessageLoop(ctx, threadID, ready, nil)
}

func runSourceRawMessageLoop(
	ctx context.Context,
	threadID uintptr,
	ready func(),
	rawReader *rawMouseReader,
	visitRawMouse func(rawInputHeader, rawMouse) error,
) error {
	return runSourceThreadMessageLoop(ctx, threadID, ready, func(msg message) error {
		if msg.Message != wmInput {
			return nil
		}
		if err := rawReader.readCurrent(msg.LParam, visitRawMouse); err != nil {
			return err
		}
		return rawReader.drainBuffered(visitRawMouse)
	})
}

func runSourceThreadMessageLoop(
	ctx context.Context,
	threadID uintptr,
	ready func(),
	visitMessage func(message) error,
) error {
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
	defer func() {
		close(stopMessageLoop)
		<-stopMessageLoopDone
	}()

	var msg message
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(result) {
		case -1:
			return fmt.Errorf("GetMessageW failed: %w", callErr)
		case 0:
			return nil
		default:
			if msg.Message == wmSourceReady {
				if ready != nil {
					ready()
					ready = nil
				}
				continue
			}
			if visitMessage != nil {
				if err := visitMessage(msg); err != nil {
					return err
				}
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func callNextHook(code int, wParam, lParam uintptr) uintptr {
	result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return result
}
