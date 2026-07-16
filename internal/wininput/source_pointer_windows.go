//go:build windows

package wininput

import (
	"fmt"
	"sync/atomic"

	"github.com/0disoft/krasis/internal/control"
)

const (
	rawMouseLeftButtonDown   = 0x0001
	rawMouseLeftButtonUp     = 0x0002
	rawMouseRightButtonDown  = 0x0004
	rawMouseRightButtonUp    = 0x0008
	rawMouseMiddleButtonDown = 0x0010
	rawMouseMiddleButtonUp   = 0x0020
	rawMouseButton4Down      = 0x0040
	rawMouseButton4Up        = 0x0080
	rawMouseButton5Down      = 0x0100
	rawMouseButton5Up        = 0x0200
	maximumRawMouseActions   = 11
)

type rawMouseActions struct {
	values [maximumRawMouseActions]control.InputAction
	count  int
}

var rawMouseButtonMappings = [...]struct {
	flag uint16
	kind control.InputActionKind
	code uint32
}{
	{rawMouseLeftButtonDown, control.InputButtonDown, MouseButtonLeft},
	{rawMouseLeftButtonUp, control.InputButtonUp, MouseButtonLeft},
	{rawMouseRightButtonDown, control.InputButtonDown, MouseButtonRight},
	{rawMouseRightButtonUp, control.InputButtonUp, MouseButtonRight},
	{rawMouseMiddleButtonDown, control.InputButtonDown, MouseButtonMiddle},
	{rawMouseMiddleButtonUp, control.InputButtonUp, MouseButtonMiddle},
	{rawMouseButton4Down, control.InputButtonDown, MouseButtonX1},
	{rawMouseButton4Up, control.InputButtonUp, MouseButtonX1},
	{rawMouseButton5Down, control.InputButtonDown, MouseButtonX2},
	{rawMouseButton5Up, control.InputButtonUp, MouseButtonX2},
}

func (actions *rawMouseActions) append(action control.InputAction) error {
	if err := action.Validate(); err != nil {
		return fmt.Errorf("map raw mouse input: %w", err)
	}
	if actions.count >= len(actions.values) {
		return fmt.Errorf("raw mouse packet exceeds %d normalized actions", len(actions.values))
	}
	actions.values[actions.count] = action
	actions.count++
	return nil
}

type sourcePointerState struct {
	x     atomic.Int32
	known atomic.Bool
}

func newSourcePointerState(position point) *sourcePointerState {
	state := &sourcePointerState{}
	state.observe(position)
	return state
}

func (state *sourcePointerState) observe(position point) {
	if state == nil {
		return
	}
	state.x.Store(position.X)
	state.known.Store(true)
}

func (state *sourcePointerState) pointerX() (int32, bool) {
	if state == nil || !state.known.Load() {
		return 0, false
	}
	return state.x.Load(), true
}

func isInjectedMouse(data mouseHookData) bool {
	return data.Flags&llmhfInjected != 0 ||
		isTaggedMouseInput(data)
}

func isTaggedMouseInput(data mouseHookData) bool {
	return data.ExtraInfo == krasisInjectedExtraInfo || data.ExtraInfo == selfTestExtraInfo
}

func shouldSuppressPhysicalMouseInput(session *SourceSession, data mouseHookData) bool {
	return !isInjectedMouse(data) && session != nil && session.Active()
}

func observeActivePhysicalHookMouse(
	observer SourcePipelineObserver,
	message uint32,
	activePhysical bool,
) {
	if observer == nil || !activePhysical {
		return
	}
	switch message {
	case wmMouseMove:
		observer.ObserveActiveHookMouseMove()
	case wmLButtonDown, wmLButtonUp,
		wmRButtonDown, wmRButtonUp,
		wmMButtonDown, wmMButtonUp,
		wmXButtonDown, wmXButtonUp:
		observer.ObserveActiveHookMouseButton()
	}
}

func mapHookMouseWheel(message uint32, data mouseHookData) (control.InputAction, bool, error) {
	if isTaggedMouseInput(data) {
		return control.InputAction{}, false, nil
	}
	delta := int32(int16(data.MouseData >> 16))
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
		return control.InputAction{}, false, fmt.Errorf("map hook mouse wheel: %w", err)
	}
	return action, true, nil
}

func mapObservedHookMouseWheel(observer SourcePipelineObserver, message uint32, data mouseHookData) (control.InputAction, bool, error) {
	observeHookMouseWheel(observer, data)
	action, emitted, err := mapHookMouseWheel(message, data)
	if observer != nil && emitted {
		observer.ObserveWheelMapped()
	}
	return action, emitted, err
}

func observeHookMouseWheel(observer SourcePipelineObserver, data mouseHookData) {
	if observer == nil || isTaggedMouseInput(data) {
		return
	}
	if data.Flags&llmhfInjected != 0 {
		observer.ObserveWheelSynthesized()
	} else {
		observer.ObserveWheelHook()
	}
}

func sourceWindowOwnsHookWheel(capture SourceActivation, data mouseHookData) (owned bool, suppress bool) {
	if capture == nil || !capture.Active() {
		return false, false
	}
	if ownership, ok := capture.(SourceWheelOwnership); ok && !ownership.OwnsWheel() {
		return false, false
	}
	return true, isTaggedMouseInput(data)
}

func mapRawMouse(mouse rawMouse) (rawMouseActions, error) {
	if uintptr(mouse.ExtraInformation) == krasisInjectedExtraInfo || uintptr(mouse.ExtraInformation) == selfTestExtraInfo {
		return rawMouseActions{}, nil
	}
	var actions rawMouseActions
	if mouse.Flags&mouseMoveAbsolute == 0 && (mouse.LastX != 0 || mouse.LastY != 0) {
		if err := actions.append(control.InputAction{Kind: control.InputPointerMove, DeltaX: mouse.LastX, DeltaY: mouse.LastY}); err != nil {
			return rawMouseActions{}, err
		}
	}
	buttonFlags := uint16(mouse.Buttons)
	for _, mapping := range rawMouseButtonMappings {
		if buttonFlags&mapping.flag == 0 {
			continue
		}
		if err := actions.append(control.InputAction{Kind: mapping.kind, Code: mapping.code}); err != nil {
			return rawMouseActions{}, err
		}
	}
	return actions, nil
}

func observeActiveRawMouseWheel(session *SourceSession, mouse rawMouse) {
	if session == nil || session.observer == nil || !session.Active() ||
		uintptr(mouse.ExtraInformation) == krasisInjectedExtraInfo ||
		uintptr(mouse.ExtraInformation) == selfTestExtraInfo {
		return
	}
	buttonFlags := uint16(mouse.Buttons)
	if int16(uint16(mouse.Buttons>>16)) == 0 {
		return
	}
	if buttonFlags&rawMouseWheel != 0 {
		session.observer.ObserveWheelRaw()
	}
	if buttonFlags&rawMouseHWheel != 0 {
		session.observer.ObserveWheelRaw()
	}
}

func routeRawMouse(
	session *SourceSession,
	boundary BoundaryObserver,
	state *sourcePointerState,
	_ rawInputHeader,
	mouse rawMouse,
) error {
	observeActiveRawMouseWheel(session, mouse)
	actions, err := mapRawMouse(mouse)
	if err != nil {
		if session != nil && session.Active() {
			session.Abort(err)
		}
		return err
	}
	if actions.count == 0 || session == nil {
		return nil
	}
	pointerX, known := state.pointerX()
	for index := 0; index < actions.count; index++ {
		action := actions.values[index]
		if !known {
			if session.Active() {
				session.Offer(action)
			}
			continue
		}
		routePhysicalAction(session, boundary, action, pointerX)
	}
	return nil
}
