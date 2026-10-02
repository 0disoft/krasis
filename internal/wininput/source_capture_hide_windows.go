//go:build windows

package wininput

const sourceCaptureHideFlags = 0x0080 | 0x0010 | 0x0004 | 0x0002 | 0x0001

var (
	callSourceCaptureHide                   = user32.NewProc("SetWindowPos").Call
	sourceWheelCaptureHideWithoutActivation = func(window uintptr) error {
		// SWP_HIDEWINDOW | NOACTIVATE | NOZORDER | NOMOVE | NOSIZE.
		hidden, _, callErr := callSourceCaptureHide(window, 0, 0, 0, 0, 0, sourceCaptureHideFlags)
		if hidden == 0 {
			return touchpadWin32Error("SetWindowPos(source capture hide)", callErr)
		}
		return nil
	}
)

func (current *sourceWheelCaptureRun) hideAfterFocusRestore() error {
	foreground := sourceWheelCaptureGetForeground()
	if foreground != 0 && foreground != current.window {
		// An external owner is already observed. Do not ask SW_HIDE to
		// choose another active window after the explicit focus restoration.
		return sourceWheelCaptureHideWithoutActivation(current.window)
	}
	// Capture still owns focus, or no owner is observable: retain the normal
	// Windows hide/release path. Never leave a hidden capture as foreground.
	sourceWheelCaptureShowWindow(current.window, swHide)
	return nil
}
