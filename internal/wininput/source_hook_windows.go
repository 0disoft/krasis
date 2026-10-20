//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
	"unsafe"
)

const wmSourceReady = 0x8002

type SourceHook struct {
	loginAuthorize     func() error   // Fixed service-child Winlogon mode only.
	interactiveSource  bool           // Fixed current interactive desktop, never follows with live owners.
	contextMenu        atomic.Uintptr // Foreground owner of the most recent local right-click release.
	boundaryGuard      func() error
	timing             HookTimingObserver
	observer           SourcePipelineObserver
	presentationReport SourcePresentationReporter
	admissionReport    func(string)
	physicalSession    atomic.Pointer[SourceSession]
	capture            *sourceWheelCapture
	returnCursor       returnCursorShield
}

type sourceInputThreadResult struct {
	component string
	err       error
	finished  time.Time
}

func NewSourceHook(timing ...HookTimingObserver) *SourceHook {
	hook := NewSourceHookObserved(nil)
	if len(timing) > 0 {
		hook.timing = timing[0]
	}
	return hook
}

func NewSourceHookObserved(observer SourcePipelineObserver) *SourceHook {
	return &SourceHook{observer: observer, capture: newSourceWheelCapture(observer)}
}

// Configure before Run; diagnostics contain fixed outcomes and durations only.
func (hook *SourceHook) SetPresentationReporter(report SourcePresentationReporter) {
	hook.presentationReport = report
	hook.capture.presentationReport = report
	hook.returnCursor.report = report
}

// Configure before Run. Reports distinguish native-only from hook-observed
// held state with fixed labels; no key/button identity or input content escapes.
func (hook *SourceHook) SetAdmissionReporter(report func(string)) { hook.admissionReport = report }

func (hook *SourceHook) SourceCapture() SourceActivation {
	if hook == nil {
		return nil
	}
	if hook.boundaryGuard != nil {
		return &guardedSourceActivation{SourceActivation: hook.capture, check: hook.boundaryGuard}
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
) (resultErr error) {
	if hook != nil && hook.loginAuthorize != nil {
		return hook.onDesktop(ctx, func(owned context.Context) error {
			return hook.runWithBoundary(owned, session, boundary, ready)
		})
	}
	return hook.runWithBoundary(ctx, session, boundary, ready)
}

func (hook *SourceHook) runWithBoundary(ctx context.Context, session *SourceSession, boundary BoundaryObserver, ready func()) (resultErr error) {
	if hook == nil || hook.capture == nil || session == nil {
		return errors.New("source hook and session are required")
	}
	if ctx == nil {
		return errors.New("source hook context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	evidence := &sourceShutdownEvidence{}
	ctx = context.WithValue(ctx, sourceShutdownKey{}, evidence)
	defer func() { resultErr = errors.Join(resultErr, evidence.snapshot()) }()
	releaseHookSession, err := acquireHookSession(ctx)
	if err != nil {
		return err
	}
	releaseAllowed := true
	hook.physicalSession.Store(session)
	defer hook.physicalSession.CompareAndSwap(session, nil)
	defer func() {
		if releaseAllowed {
			releaseHookSession()
		}
	}()
	initialPosition, err := cursorPosition()
	if err != nil {
		return sourceStage("pointer-snapshot", err)
	}
	hook.contextMenu.Store(0) // A gesture from an earlier connection is not popup evidence.
	pointerState := newSourcePointerState(initialPosition)
	timing := newHookTimingRelay(hook.timing)
	defer timing.Close()
	observations := newSourceObservationRelay(hook.observer, nil, hook.presentationReport)
	defer observations.Close()
	hook.capture.presentationReport = observations.presentationReporter()
	hook.returnCursor.report = observations.presentationReporter()
	progress := &sourceHookProgress{origin: time.Now(), next: timing}
	pointerState.progress = progress
	session.physicalMu.Lock()
	session.physicalCheck = sourceAnyInputPressed
	session.advancePhysicalRevision()
	session.physicalMu.Unlock()
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	readySignals := make(chan struct{}, 3)
	results := make(chan sourceInputThreadResult, 3)
	startSourceInputThread(results, "low-level-hook", func() error {
		return hook.onDesktop(runCtx, func(owned context.Context) error {
			return runSourceLowLevelHookThread(owned, session, boundary, observations.Pipeline(), hook.capture, pointerState, progress, func() {
				readySignals <- struct{}{}
			}, &hook.contextMenu, &hook.returnCursor)
		})
	})
	startSourceInputThread(results, "raw-mouse", func() error {
		return hook.onDesktop(runCtx, func(owned context.Context) error {
			return runSourceRawMouseThread(owned, session, boundary, pointerState, progress, func() {
				readySignals <- struct{}{}
			})
		})
	})
	startSourceInputThread(results, "wheel-capture", func() error {
		return hook.onDesktop(runCtx, func(owned context.Context) error {
			return hook.capture.runWindow(owned, session, func() {
				readySignals <- struct{}{}
			})
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
	watch := time.NewTicker(100 * time.Millisecond)
	defer watch.Stop()
	for {
		select {
		case <-watch.C:
			if err := progress.failure(time.Now()); err != nil {
				session.retireHookFailure(err)
				resultErr, joined := collectSourceInputThreads(ctx, cancel, results, nil, 3, SourceCleanupTimeout)
				releaseAllowed = joined
				if joined && cancellationOnly(resultErr) {
					return &retiredHookProgress{cause: errors.Join(err, resultErr)}
				}
				return errors.Join(err, resultErr)
			}
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
}

func startSourceInputThread(results chan<- sourceInputThreadResult, component string, run func() error) {
	go func() {
		runtime.LockOSThread()
		err := run()
		results <- sourceInputThreadResult{component: component, err: err, finished: time.Now()}
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
	deadline := time.Now().Add(timeout)
	cancel()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	late := false
	accept := func(result sourceInputThreadResult) {
		late = late || (!result.finished.IsZero() && result.finished.After(deadline))
		collected = append(collected, result)
	}

join:
	for len(collected) < expected {
		select {
		case result := <-results:
			accept(result)
		case <-timer.C:
			// Scheduling can deliver the timer alongside completions queued
			// before its deadline. Drain those before declaring a live owner.
			for len(collected) < expected {
				select {
				case result := <-results:
					accept(result)
				default:
					break join
				}
			}
		}
	}
	if len(collected) < expected || late {
		stage := sourceThreadJoinStage(collected, expected)
		collected = append(collected, sourceInputThreadResult{component: stage, err: errSourceInputJoinTimeout})
		return sourceInputThreadsError(parent, collected...), false
	}
	return sourceInputThreadsError(parent, collected...), true
}

func sourceThreadJoinStage(collected []sourceInputThreadResult, expected int) string {
	if expected != 3 {
		return "thread-join"
	}
	missing := ""
	for _, component := range []string{"low-level-hook", "raw-mouse", "wheel-capture"} {
		found := false
		for _, result := range collected {
			found = found || result.component == component
		}
		if !found {
			if missing != "" {
				return "thread-join-multiple"
			}
			missing = "thread-join-" + component
		}
	}
	if missing == "" {
		return "thread-join" // All returned, but at least one missed its deadline.
	}
	return missing
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
			joined = append(joined, sourceWorkerStage(result.component, fmt.Errorf("source %s stopped unexpectedly", result.component)))
			continue
		}
		joined = append(joined, sourceWorkerStage(result.component, result.err))
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
	timing HookTimingObserver,
	ready func(),
	contextMenu *atomic.Uintptr,
	shields ...*returnCursorShield,
) (resultErr error) {
	var shield *returnCursorShield
	if len(shields) != 0 {
		shield = shields[0]
		shield.loginContext = nil
		if sourceLoginScope(ctx) != nil {
			shield.loginContext = ctx
		}
		shield.open()
		defer func() { resultErr = errors.Join(resultErr, shield.close()) }()
	}
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)
	callbacks := newSourceLowLevelCallbacks(session, boundary, observer, wheelCapture, pointerState, contextMenu, shield,
		func() error { return sourceLoginDesktopCheck(ctx) }, callNextHook)
	owner := &sourceHookCallbacks{keyboard: timedSourceCallback(callbacks.keyboard, timing), mouse: timedSourceCallback(callbacks.mouse, timing)}
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
		return sourceStage("hook-install", fmt.Errorf("SetWindowsHookExW(WH_KEYBOARD_LL) failed: %w", callErr))
	}
	mouseHook, _, callErr := procSetWindowsHookExW.Call(whMouseLL, sourceMouseCallback, module, 0)
	if mouseHook == 0 {
		procUnhookWindowsHookEx.Call(keyboardHook)
		return sourceStage("hook-install", fmt.Errorf("SetWindowsHookExW(WH_MOUSE_LL) failed: %w", callErr))
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
		return sourceStage("hook-release", cleanupErr)
	}
	defer cleanupHooks()
	if err := postSourceReady(threadID); err != nil {
		return err
	}
	loopErr := runSourceHookMessageLoop(ctx, ready)
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
	timing HookTimingObserver,
	ready func(),
) (resultErr error) {
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)
	window, err := createRawInputMessageWindow("KrasisSourceRawMouse")
	if err != nil {
		return sourceStage("raw-window-create", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, sourceStage("raw-window-destroy", destroyRawInputMessageWindow(window)))
	}()
	if err := registerRawMouseInput(window); err != nil {
		return sourceStage("raw-register", err)
	}
	rawRegistered := true
	cleanupRawInput := func() error {
		if !rawRegistered {
			return nil
		}
		rawRegistered = false
		return sourceStage("raw-release", unregisterRawMouseInput())
	}
	defer func() { resultErr = errors.Join(resultErr, cleanupRawInput()) }()
	rawReader, err := newRawMouseReader()
	if err != nil {
		return err
	}
	if err := postSourceReady(threadID); err != nil {
		return sourceStage("raw-ready", err)
	}
	visitRawMouse := func(header rawInputHeader, mouse rawMouse) error {
		if err := sourceLoginDesktopCheck(ctx); err != nil {
			return sourceWorkerStage("raw-desktop", err)
		}
		if timing != nil {
			timing.ObserveRawInput(time.Now())
		}
		if pointerState != nil && pointerState.progress != nil && mouse.Flags&mouseMoveAbsolute == 0 &&
			(mouse.LastX != 0 || mouse.LastY != 0) &&
			uintptr(mouse.ExtraInformation) != krasisInjectedExtraInfo && uintptr(mouse.ExtraInformation) != selfTestExtraInfo {
			pointerState.progress.observePhysicalRaw(time.Now())
		}
		return sourceStage("raw-route", routeRawMouse(session, boundary, pointerState, header, mouse))
	}
	loopErr := runSourceRawMessageLoop(ctx, ready, rawReader, visitRawMouse)
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

func runSourceHookMessageLoop(ctx context.Context, ready func()) error {
	return runSourceThreadMessageLoop(ctx, ready, nil)
}

func runSourceRawMessageLoop(
	ctx context.Context,
	ready func(),
	rawReader *rawMouseReader,
	visitRawMouse func(rawInputHeader, rawMouse) error,
) error {
	return runSourceThreadMessageLoop(ctx, ready, func(msg message) error {
		if msg.Message != wmInput {
			return nil
		}
		if err := rawReader.readCurrentContext(ctx, msg.LParam, visitRawMouse); err != nil {
			return sourceStage("raw-read", err)
		}
		return sourceStage("raw-buffer", rawReader.drainBuffered(visitRawMouse))
	})
}

func runSourceThreadMessageLoop(
	ctx context.Context,
	ready func(),
	visitMessage func(message) error,
) error {
	return sourceMessageLoop(ctx, func(msg message) error {
		if msg.Message == wmSourceReady {
			if ready != nil {
				ready()
				ready = nil
			}
			return nil
		}
		if visitMessage != nil {
			return visitMessage(msg)
		}
		return nil
	})
}

func callNextHook(code int, wParam, lParam uintptr) uintptr {
	result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return result
}
