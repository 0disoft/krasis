//go:build windows

package wininput

import "github.com/0disoft/krasis/internal/control"

func mapSourceHookButton(message uint32, data mouseHookData) (control.InputAction, bool) {
	if isInjectedMouse(data) {
		return control.InputAction{}, false
	}
	action := control.InputAction{Kind: control.InputButtonDown}
	switch message {
	case wmLButtonDown, wmLButtonUp:
		action.Code = MouseButtonLeft
	case wmRButtonDown, wmRButtonUp:
		action.Code = MouseButtonRight
	case wmMButtonDown, wmMButtonUp:
		action.Code = MouseButtonMiddle
	case wmXButtonDown, wmXButtonUp:
		switch uint16(data.MouseData >> 16) {
		case 1:
			action.Code = MouseButtonX1
		case 2:
			action.Code = MouseButtonX2
		default:
			return control.InputAction{}, false
		}
	default:
		return control.InputAction{}, false
	}
	switch message {
	case wmLButtonUp, wmRButtonUp, wmMButtonUp, wmXButtonUp:
		action.Kind = control.InputButtonUp
	}
	return action, true
}
