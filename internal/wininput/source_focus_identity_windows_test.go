//go:build windows

package wininput

import "testing"

func TestSavedFocusLifetimeCookieRejectsReusedWindow(t *testing.T) {
	owner, property, isWindow := sourceFocusOwner, sourceFocusProperty, sourceWheelCaptureIsWindow
	t.Cleanup(func() { sourceFocusOwner, sourceFocusProperty, sourceWheelCaptureIsWindow = owner, property, isWindow })
	pid, tid, cookie := uint32(21), uint32(22), uintptr(1)
	sourceFocusOwner = func(uintptr) (uint32, uint32) { return pid, tid }
	sourceFocusProperty = func(uintptr, []uint16, string) uintptr { return cookie }
	sourceWheelCaptureIsWindow = func(uintptr) bool { return true }
	identity := sourceFocusIdentity{name: []uint16{1, 0}, process: pid, thread: tid}
	if !sourceFocusMatches(20, identity) {
		t.Fatal("original window rejected")
	}
	// Same HWND, same owner: destruction removes the original window's cookie.
	cookie = 0
	if sourceFocusMatches(20, identity) {
		t.Fatal("same-thread HWND reuse accepted")
	}
	cookie, pid = 1, 23
	if sourceFocusMatches(20, identity) {
		t.Fatal("different process accepted")
	}
	pid, tid = 21, 24
	if sourceFocusMatches(20, identity) {
		t.Fatal("different thread accepted")
	}
	if sourceFocusMatches(20, sourceFocusIdentity{}) {
		t.Fatal("unobserved identity accepted")
	}
}

func TestIdentityLostDuringFocusProbeNeverRestores(t *testing.T) {
	cleanup := stubSourceWheelCaptureActivation(t)
	defer cleanup()
	valid := true
	sourceFocusMatches = func(uintptr, sourceFocusIdentity) bool { return valid }
	sourceWheelCaptureGetForeground = func() uintptr { return 0 }
	sourceWheelCaptureIsVisible = func(uintptr) bool { return true }
	sourceRestoreWindowEnabled = func(uintptr) bool { return true }
	sourceWheelCaptureProbeForeground = func(uintptr) error { valid = false; return nil }
	sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("reused window activated"); return nil }
	current := &sourceWheelCaptureRun{window: 10, savedFocus: 20}
	current.restoreAfterHideProfile(nil, SourceFocusRestoreAPIRejected, ErrSourceFocusRestoreAPIRejected)
}
