//go:build windows

package wininput

import "github.com/0disoft/krasis/internal/control"

const llkhfExtended = 0x00000001

type sourceKeyboardMapper struct{}

func (mapper *sourceKeyboardMapper) keyboard(message uint32, data keyboardHookData) (control.InputAction, bool, error) {
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
