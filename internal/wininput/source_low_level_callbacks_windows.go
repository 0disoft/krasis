//go:build windows

package wininput

import (
	"sync/atomic"
	"unsafe"

	"github.com/0disoft/krasis/internal/control"
)

// Native owner callbacks retain only physical-input work on the capture gate.
// check and next are the existing desktop guard and hook-chain continuation;
// injected tags never grant authority or suppress another application's input.
func newSourceLowLevelCallbacks(session *SourceSession, boundary BoundaryObserver,
	observer SourcePipelineObserver, wheelCapture SourceActivation, pointerState *sourcePointerState,
	contextMenu *atomic.Uintptr, shield *returnCursorShield, check func() error, next sourceHookCallback) *sourceHookCallbacks {
	mapper := &sourceKeyboardMapper{}
	keyboardCallback := func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			data := *(*keyboardHookData)(unsafe.Pointer(lParam))
			if isInjectedKeyboard(data) {
				if err := shield.beforeIgnoredInput(check); err != nil {
					session.Abort(err)
				}
				return next(code, wParam, lParam)
			}
			if err := check(); err != nil {
				session.Abort(err)
				return next(code, wParam, lParam)
			}
			if err := shield.beforeInput(); err != nil {
				session.Abort(err)
				return next(code, wParam, lParam)
			}
			if routeMappedPhysicalAction(session, boundary, func() (control.InputAction, bool, int32, error) {
				action, emitted, err := mapper.keyboard(uint32(wParam), data)
				if emitted && contextMenu != nil {
					contextMenu.Store(0)
				}
				return action, emitted, 0, err
			}) {
				return 1
			}
		}
		return next(code, wParam, lParam)
	}
	mouseCallback := func(code int, wParam uintptr, lParam uintptr) uintptr {
		if code == hcAction && lParam != 0 {
			data := *(*mouseHookData)(unsafe.Pointer(lParam))
			if pointerState != nil && pointerState.progress != nil && !isInjectedMouse(data) {
				pointerState.progress.observeMouse()
			}
			message := uint32(wParam)
			// Wheels retain existing foreground ownership/suppression. Untagged
			// synthesized wheels can also come from precision touchpads.
			if isTaggedMouseInput(data) && message != wmMouseWheel && message != wmMouseHWheel {
				if err := shield.beforeIgnoredMouseInput(message, data.Point, check); err != nil {
					session.Abort(err)
				} else if message == wmMouseMove {
					pointerState.observe(data.Point)
				}
				return next(code, wParam, lParam)
			}
			if err := check(); err != nil {
				session.Abort(err)
				return next(code, wParam, lParam)
			}
			if err := shield.beforeMouseInput(message, data.Point); err != nil {
				session.Abort(err)
				return next(code, wParam, lParam)
			}
			if action, button := mapSourceHookButton(message, data); button {
				if contextMenu != nil {
					owner := uintptr(0)
					if action.Kind == control.InputButtonUp && action.Code == MouseButtonRight && !session.Active() {
						owner = sourceWheelCaptureGetForeground()
					}
					contextMenu.Store(owner)
				}
				if routePhysicalAction(session, boundary, action, data.Point.X) {
					return 1
				}
				return next(code, wParam, lParam)
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
					return next(code, wParam, lParam)
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
		return next(code, wParam, lParam)
	}
	return &sourceHookCallbacks{keyboard: keyboardCallback, mouse: mouseCallback}
}
