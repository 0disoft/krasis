//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const wmSourceReady = 0x8002

type SourceHook struct{}

func NewSourceHook() *SourceHook {
	return &SourceHook{}
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
	if hook == nil || session == nil {
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
	defer releaseHookSession()
	completed := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		completed <- runSourceHookOnDedicatedThread(ctx, session, boundary, ready)
	}()
	return <-completed
}

func runSourceHookOnDedicatedThread(
	ctx context.Context,
	session *SourceSession,
	boundary BoundaryObserver,
	ready func(),
) error {
	threadID, _, _ := procGetCurrentThreadID.Call()
	var bootstrapMessage message
	procPeekMessageW.Call(uintptr(unsafe.Pointer(&bootstrapMessage)), 0, 0, 0, pmNoRemove)
	mapper := &sourceMapper{}
	keyboardCallback := syscall.NewCallback(func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			data := *(*keyboardHookData)(unsafe.Pointer(lParam))
			action, emitted, err := mapper.keyboard(uint32(wParam), data)
			if err != nil {
				if session.Active() {
					session.Abort(err)
				}
			} else if emitted && routePhysicalAction(session, boundary, action, 0) {
				return 1
			}
		}
		return callNextHook(code, wParam, lParam)
	})
	mouseCallback := syscall.NewCallback(func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			data := *(*mouseHookData)(unsafe.Pointer(lParam))
			action, emitted, err := mapper.mouse(uint32(wParam), data)
			if err != nil {
				if session.Active() {
					session.Abort(err)
				}
			} else if emitted && routePhysicalAction(session, boundary, action, data.Point.X) {
				return 1
			}
		}
		return callNextHook(code, wParam, lParam)
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
	if ready != nil {
		result, _, postErr := procPostThreadMessageW.Call(threadID, wmSourceReady, 0, 0)
		if result == 0 {
			return fmt.Errorf("PostThreadMessageW(source ready) failed: %w", postErr)
		}
	}

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
	loopErr := runSourceMessageLoop(ready)
	unhookErr := cleanupHooks()
	close(stopMessageLoop)
	<-stopMessageLoopDone
	if loopErr != nil {
		return loopErr
	}
	if unhookErr != nil {
		return unhookErr
	}
	return ctx.Err()
}

func runSourceMessageLoop(ready func()) error {
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
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func callNextHook(code int, wParam, lParam uintptr) uintptr {
	result, _, _ := procCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return result
}
