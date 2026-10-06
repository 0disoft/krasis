//go:build windows

package wininput

import (
	"errors"
	"reflect"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

func TestFullscreenClientGeometryExcludesMaximizedWorkArea(t *testing.T) {
	for _, test := range []struct {
		name    string
		client  fullscreenRect
		origin  point
		monitor fullscreenRect
		want    bool
	}{
		{"fullscreen", fullscreenRect{0, 0, 1920, 1080}, point{0, 0}, fullscreenRect{0, 0, 1920, 1080}, true},
		{"taskbar visible", fullscreenRect{0, 0, 1920, 1040}, point{0, 0}, fullscreenRect{0, 0, 1920, 1080}, false},
		{"titlebar", fullscreenRect{0, 0, 1920, 1048}, point{0, 32}, fullscreenRect{0, 0, 1920, 1080}, false},
		{"negative monitor", fullscreenRect{0, 0, 1600, 900}, point{-1600, -100}, fullscreenRect{-1600, -100, 0, 800}, true},
		{"scaled monitor", fullscreenRect{0, 0, 2560, 1440}, point{0, 0}, fullscreenRect{0, 0, 2560, 1440}, true},
		{"empty", fullscreenRect{}, point{}, fullscreenRect{0, 0, 1920, 1080}, false},
		{"invalid monitor", fullscreenRect{0, 0, 1920, 1080}, point{}, fullscreenRect{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := clientCoversFullscreenMonitor(test.client, test.origin, test.monitor); got != test.want {
				t.Fatal(got, test.want)
			}
		})
	}
}

func TestNativeFullscreenShellMarkForHiddenOwnedWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := syscall.UTF16PtrFromString("STATIC")
	title, _ := syscall.UTF16PtrFromString("Krasis hidden fullscreen policy test")
	window, _, err := procCreateWindowExW.Call(wsExToolWindow, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 1, 1, 0, 0, 0, 0)
	if window == 0 {
		t.Fatal("create hidden test window", err)
	}
	defer procDestroyWindow.Call(window)
	presentation := &sourcePresentationState{window: window}
	sourcePresentationWindows.Store(window, presentation)
	defer sourcePresentationWindows.Delete(window)
	if !setSourceCaptureNormalHint(window, true) {
		t.Fatal("native normal-window hint rejected")
	}
	property := user32.NewProc("GetPropW")
	if value, _, _ := property.Call(window, uintptr(unsafe.Pointer(sourceNormalHintName))); value != 1 {
		t.Fatal("normal-window hint was not set")
	}
	if !setSourceCaptureNormalHint(window, false) {
		t.Fatal("normal-window hint removal rejected")
	}
	if value, _, _ := property.Call(window, uintptr(unsafe.Pointer(sourceNormalHintName))); value != 0 {
		t.Fatal("normal-window hint remained set")
	}
	if !markSourceCaptureFullscreen(window, true) {
		t.Fatal("native fullscreen mark rejected")
	}
	if !markSourceCaptureFullscreen(window, false) {
		t.Fatal("native fullscreen clearing rejected")
	}
	t.Log("ITaskbarList2 mark and clear accepted on a hidden owned HWND")
}

func TestCapturePreservesFullscreenTaskbarPolicyAcrossActivationAndReturn(t *testing.T) {
	for _, fullscreen := range []bool{false, true} {
		restore := stubSourceWheelCaptureActivation(t)
		foreground := uintptr(20)
		events := []string{}
		sourceWheelCaptureGetForeground = func() uintptr { return foreground }
		sourceWheelCaptureSetForeground = func(window uintptr) error { events = append(events, "focus"); foreground = window; return nil }
		sourceCaptureIsFullscreen = func(window uintptr) bool {
			if window != 20 {
				t.Fatal("inspected wrong window")
			}
			return fullscreen
		}
		sourceCaptureMarkFullscreen = func(window uintptr, enabled bool) bool {
			if window != 10 {
				t.Fatal("marked another application's window")
			}
			if enabled {
				events = append(events, "mark")
			} else {
				events = append(events, "clear")
			}
			return true
		}
		sourceWheelCaptureShowWindow = func(_ uintptr, command uintptr) {
			if command == swShowWithoutActivation {
				events = append(events, "show")
			} else {
				events = append(events, "hide")
			}
		}
		current := &sourceWheelCaptureRun{window: 10}
		if err := current.activateWindow(); err != nil {
			t.Fatal(err)
		}
		if err := current.deactivateWindow(); err != nil {
			t.Fatal(err)
		}
		want := []string{"show", "focus", "focus", "hide"}
		if fullscreen {
			want = []string{"mark", "show", "focus", "focus", "hide", "clear"}
		}
		if !reflect.DeepEqual(events, want) || current.fullscreenMarked || foreground != 20 {
			t.Fatal(events, want, current.fullscreenMarked, foreground)
		}
		restore()
	}
}

func TestFullscreenMarkIsClearedOnActivationRollback(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(20)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(uintptr) error { return errors.New("activation denied") }
	sourceWheelCaptureAttachThreads = func(uint32, uint32, bool) error { return errors.New("fallback denied") }
	sourceCaptureIsFullscreen = func(uintptr) bool { return true }
	flags := []bool{}
	sourceCaptureMarkFullscreen = func(_ uintptr, flag bool) bool { flags = append(flags, flag); return true }
	current := &sourceWheelCaptureRun{window: 10}
	if err := current.activateWindow(); err == nil {
		t.Fatal("failed activation accepted")
	}
	if !reflect.DeepEqual(flags, []bool{true, false}) || current.fullscreenMarked || current.active.Load() {
		t.Fatal(flags, current.fullscreenMarked)
	}
}

func TestFullscreenPresentationFailureDoesNotInterruptInput(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(20)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
	sourceCaptureIsFullscreen = func(uintptr) bool { return true }
	sourceCaptureMarkFullscreen = func(uintptr, bool) bool { return false }
	current := &sourceWheelCaptureRun{window: 10}
	if err := current.activateWindow(); err != nil || !current.active.Load() || current.fullscreenMarked {
		t.Fatal(err)
	}
	if err := current.deactivateWindow(); err != nil || foreground != 20 {
		t.Fatal(err, foreground)
	}
}

func TestFullscreenClearingRetriesAfterReturn(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	clears := 0
	sourceCaptureMarkFullscreen = func(window uintptr, enabled bool) bool {
		if window != 10 || enabled {
			t.Fatal("expected clearing of owned capture window")
		}
		clears++
		return clears > 1
	}
	current := &sourceWheelCaptureRun{window: 10, fullscreenMarked: true, fullscreenAttempted: true}
	if err := current.deactivateWindow(); err != nil || !current.fullscreenMarked {
		t.Fatal("failed clearing lost retry state", err)
	}
	if err := current.deactivateWindow(); err != nil || current.fullscreenMarked || clears != 2 {
		t.Fatal("repeated deactivation did not clear marking", err, clears)
	}
}

func TestNormalHintIsAppliedBeforeShowingNormalCaptureAndAfterFailedClear(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	foreground := uintptr(20)
	sourceWheelCaptureGetForeground = func() uintptr { return foreground }
	sourceWheelCaptureSetForeground = func(window uintptr) error { foreground = window; return nil }
	events := []string{}
	sourceCaptureNormalHint = func(window uintptr, normal bool) bool {
		if window != 10 || !normal {
			t.Fatal("wrong window or normal hint")
		}
		events = append(events, "normal")
		return true
	}
	sourceCaptureMarkFullscreen = func(uintptr, bool) bool { events = append(events, "failed_clear"); return false }
	sourceWheelCaptureShowWindow = func(_ uintptr, command uintptr) {
		if command == swShowWithoutActivation {
			events = append(events, "show")
		}
	}
	current := &sourceWheelCaptureRun{window: 10, fullscreenAttempted: true, fullscreenMarked: true}
	if err := current.activateWindow(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"failed_clear", "normal", "show"}) {
		t.Fatal(events)
	}
	if err := current.deactivateWindow(); err != nil {
		t.Fatal(err)
	}
	if !current.fullscreenAttempted {
		t.Fatal("failed clear discarded outstanding cleanup")
	}
}
