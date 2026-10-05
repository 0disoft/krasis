//go:build windows

package wininput

var callSourceRestoreActive = user32.NewProc("SetActiveWindow").Call
var callSourceRestoreEnabled = user32.NewProc("IsWindowEnabled").Call

var sourceWheelCaptureIsEnabled = func(window uintptr) bool {
	enabled, _, _ := callSourceRestoreEnabled(window)
	return enabled != 0
}

// A return value is the previous active HWND, not proof of foreground ownership.
// The caller observes GetForegroundWindow before and after detachment instead.
var sourceWheelCaptureSetActive = func(window uintptr) {
	callSourceRestoreActive(window)
}
