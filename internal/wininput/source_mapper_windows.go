//go:build windows

package wininput

import "github.com/0disoft/krasis/internal/control"

const llkhfExtended = 0x00000001

type sourceKeyboardMapper struct{}

func (mapper *sourceKeyboardMapper) keyboard(message uint32, data keyboardHookData) (control.InputAction, bool, error) {
	if data.Flags&llkhfInjected != 0 || data.ExtraInfo == krasisInjectedExtraInfo || data.ExtraInfo == selfTestExtraInfo {
		return control.InputAction{}, false, nil
	}
	// Right Shift has its own non-E0 scan code. Some IME paths report an
	// extended flag for it; keep down/up identity stable before held tracking.
	extended := data.Flags&llkhfExtended != 0 && data.ScanCode != scanCodeRightShift
	code, err := EncodeScanCode(uint16(data.ScanCode), extended)
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
