//go:build windows

package wininput

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/keyboardstate"
	"syscall"
	"unsafe"
)

var syscallImmDefaultWindow = syscall.NewLazyDLL("imm32.dll").NewProc("ImmGetDefaultIMEWnd").Call

var lockKeyState = user32.NewProc("GetKeyState").Call
var lockForeground = sourceWheelCaptureGetForeground
var lockKeyHeld = func(key uintptr) bool { v, _, _ := procGetAsyncKeyStateSource.Call(key); return v&0x8000 != 0 }
var lockSend = sendInputs
var lockWindowLayout = func(window uintptr) uintptr {
	thread, _, _ := procGetWindowThreadIDSourceCapture.Call(window, 0)
	if thread == 0 {
		return 0
	}
	layout, _, _ := user32.NewProc("GetKeyboardLayout").Call(thread)
	return layout
}

func lockMode(key uintptr) string {
	v, _, _ := lockKeyState(key)
	if v&1 != 0 {
		return "ON"
	}
	return "OFF"
}

// Called on the GUI message thread. IME observation is bounded and never reads
// an edit control or modifies the foreground application's IME context.
func ReadLockState(window uintptr) (v keyboardstate.State) {
	v = keyboardstate.Unknown()
	foreground := lockForeground()
	if foreground == 0 {
		return v
	}
	defer func() {
		if lockForeground() != foreground {
			v = keyboardstate.Unknown()
		}
	}()
	// GetKeyState follows this GUI queue, not a background global snapshot.
	if window != 0 && foreground == window {
		v.Caps, v.Num = lockMode(0x14), lockMode(0x90)
	}
	layout := lockWindowLayout(foreground)
	if uint16(layout) != 0x0412 {
		return v
	}
	ime, _, _ := syscallImmDefaultWindow(foreground)
	if ime == 0 {
		return v
	}
	var open uintptr
	accepted, _, _ := user32.NewProc("SendMessageTimeoutW").Call(ime, 0x0283, 5, 0, 0x23, 20, uintptr(unsafe.Pointer(&open)))
	if accepted != 0 && lockForeground() == foreground {
		v.Hangul = "OFF"
		if open != 0 {
			v.Hangul = "ON"
		}
	}
	if lockForeground() != foreground {
		return keyboardstate.Unknown()
	}
	return v
}

// SyncLocalLockState is a one-shot GUI action on this PC only. The GUI must
// still own foreground; held physical keys abort rather than change shortcuts.
// Hangul is per application and deliberately excluded from this operation.
func SyncLocalLockState(ctx context.Context, window uintptr, desired keyboardstate.State) error {
	if ctx == nil || ctx.Err() != nil || window == 0 || lockForeground() != window || !desired.LocksKnown() {
		return errors.New("lock state unavailable")
	}
	for _, key := range []uintptr{0x10, 0x11, 0x12, 0x5b, 0x5c, 0x14, 0x90} {
		if lockKeyHeld(key) {
			return errors.New("physical key held")
		}
	}
	var events, releases []input
	for _, key := range []struct {
		vk       uintptr
		scan     uint16
		extended bool
		want     string
	}{
		{0x14, 0x3a, false, desired.Caps}, {0x90, 0x45, true, desired.Num},
	} {
		if lockMode(key.vk) != key.want {
			events = append(events, keyboardEvent(key.scan, key.extended, false), keyboardEvent(key.scan, key.extended, true))
			releases = append(releases, keyboardEvent(key.scan, key.extended, true))
		}
	}
	if lockForeground() != window || ctx.Err() != nil {
		return errors.New("foreground changed")
	}
	if len(events) == 0 {
		return nil
	}
	if err := lockSend(ctx, events); err != nil {
		// Only releases, never repeat a possibly applied toggle.
		cleanup, cancel := context.WithTimeout(context.Background(), SourceCleanupTimeout)
		defer cancel()
		return errors.Join(err, lockSend(cleanup, releases))
	}
	return nil
}
