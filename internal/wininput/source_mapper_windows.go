//go:build windows

package wininput

import (
	"fmt"

	"github.com/0disoft/krasis/internal/control"
)

const llkhfExtended = 0x00000001

type sourceMapper struct {
	lastMouse    point
	hasLastMouse bool
}

func (mapper *sourceMapper) keyboard(message uint32, data keyboardHookData) (control.InputAction, bool, error) {
	if data.Flags&llkhfInjected != 0 || data.ExtraInfo == krasisInjectedExtraInfo || data.ExtraInfo == selfTestExtraInfo {
		return control.InputAction{}, false, nil
	}
	code, err := EncodeScanCode(uint16(data.ScanCode), data.Flags&llkhfExtended != 0)
	if err != nil {
		return control.InputAction{}, false, err
	}
	action := control.InputAction{Code: code}
	switch message {
	case wmKeyDown, wmSysKeyDown:
		action.Kind = control.InputKeyDown
	case wmKeyUp, wmSysKeyUp:
		action.Kind = control.InputKeyUp
	default:
		return control.InputAction{}, false, nil
	}
	return action, true, nil
}

func (mapper *sourceMapper) mouse(message uint32, data mouseHookData) (control.InputAction, bool, error) {
	if message == wmMouseMove {
		previous := mapper.lastMouse
		hadPrevious := mapper.hasLastMouse
		mapper.lastMouse = data.Point
		mapper.hasLastMouse = true
		if data.Flags&llmhfInjected != 0 || data.ExtraInfo == krasisInjectedExtraInfo || data.ExtraInfo == selfTestExtraInfo || !hadPrevious {
			return control.InputAction{}, false, nil
		}
		deltaX := data.Point.X - previous.X
		deltaY := data.Point.Y - previous.Y
		if deltaX == 0 && deltaY == 0 {
			return control.InputAction{}, false, nil
		}
		return control.InputAction{Kind: control.InputPointerMove, DeltaX: deltaX, DeltaY: deltaY}, true, nil
	}
	if data.Flags&llmhfInjected != 0 || data.ExtraInfo == krasisInjectedExtraInfo || data.ExtraInfo == selfTestExtraInfo {
		return control.InputAction{}, false, nil
	}
	action := control.InputAction{}
	switch message {
	case wmLButtonDown:
		action = control.InputAction{Kind: control.InputButtonDown, Code: MouseButtonLeft}
	case wmLButtonUp:
		action = control.InputAction{Kind: control.InputButtonUp, Code: MouseButtonLeft}
	case wmRButtonDown:
		action = control.InputAction{Kind: control.InputButtonDown, Code: MouseButtonRight}
	case wmRButtonUp:
		action = control.InputAction{Kind: control.InputButtonUp, Code: MouseButtonRight}
	case wmMButtonDown:
		action = control.InputAction{Kind: control.InputButtonDown, Code: MouseButtonMiddle}
	case wmMButtonUp:
		action = control.InputAction{Kind: control.InputButtonUp, Code: MouseButtonMiddle}
	case wmXButtonDown, wmXButtonUp:
		code, err := xButtonCode(data.MouseData)
		if err != nil {
			return control.InputAction{}, false, err
		}
		action = control.InputAction{Kind: control.InputButtonDown, Code: code}
		if message == wmXButtonUp {
			action.Kind = control.InputButtonUp
		}
	case wmMouseWheel:
		action = control.InputAction{Kind: control.InputWheel, DeltaY: int32(wheelDelta(data.MouseData))}
	case wmMouseHWheel:
		action = control.InputAction{Kind: control.InputWheel, DeltaX: int32(wheelDelta(data.MouseData))}
	default:
		return control.InputAction{}, false, nil
	}
	if err := action.Validate(); err != nil {
		return control.InputAction{}, false, fmt.Errorf("map physical mouse input: %w", err)
	}
	return action, true, nil
}

func wheelDelta(mouseData uint32) int16 {
	return int16(mouseData >> 16)
}

func xButtonCode(mouseData uint32) (uint32, error) {
	switch uint16(mouseData >> 16) {
	case xButton1:
		return MouseButtonX1, nil
	case xButton2:
		return MouseButtonX2, nil
	default:
		return 0, fmt.Errorf("%w: unknown X button", ErrUnsupportedInput)
	}
}
