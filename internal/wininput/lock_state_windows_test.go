//go:build windows

package wininput

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/keyboardstate"
	"testing"
	"unsafe"
)

func TestBackgroundGUIQueueNeverAdvertisesCurrentLockState(t *testing.T) {
	get, foreground, layout := lockKeyState, lockForeground, lockWindowLayout
	t.Cleanup(func() { lockKeyState, lockForeground, lockWindowLayout = get, foreground, layout })
	lockKeyState = func(...uintptr) (uintptr, uintptr, error) {
		t.Fatal("background queue sampled as current")
		return 0, 0, nil
	}
	lockForeground = func() uintptr { return 20 }
	lockWindowLayout = func(uintptr) uintptr { return 0 }
	if got := ReadLockState(10); got != keyboardstate.Unknown() {
		t.Fatal(got)
	}
}

func TestLockMatchingRequiresForegroundKnownStateAndReleasedKeys(t *testing.T) {
	get, foreground, held, send := lockKeyState, lockForeground, lockKeyHeld, lockSend
	t.Cleanup(func() { lockKeyState, lockForeground, lockKeyHeld, lockSend = get, foreground, held, send })
	lockKeyState = func(...uintptr) (uintptr, uintptr, error) { return 0, 0, nil }
	lockForeground = func() uintptr { return 10 }
	lockKeyHeld = func(uintptr) bool { return false }
	calls := 0
	lockSend = func(_ context.Context, events []input) error {
		calls++
		if len(events) != 2 {
			t.Fatal("expected just Caps down/up")
		}
		for i := range events {
			key := (*keyboardInput)(unsafe.Pointer(&events[i].Mouse))
			if key.ScanCode != 0x3a || (key.Flags&keyEventKeyUp != 0) != (i == 1) {
				t.Fatal("incorrect lock pair")
			}
		}
		return nil
	}
	want := keyboardstate.State{Caps: "ON", Num: "OFF", Hangul: "ON"}
	if err := SyncLocalLockState(context.Background(), 10, want); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
	if err := SyncLocalLockState(context.Background(), 20, want); err == nil {
		t.Fatal("foreign foreground accepted")
	}
	if err := SyncLocalLockState(context.Background(), 10, keyboardstate.Unknown()); err == nil {
		t.Fatal("unknown desired state accepted")
	}
	lockKeyHeld = func(uintptr) bool { return true }
	if err := SyncLocalLockState(context.Background(), 10, want); err == nil || calls != 1 {
		t.Fatal("held keys were changed")
	}
}

func TestPartialLockMatchingReleasesWithoutRetoggling(t *testing.T) {
	get, foreground, held, send := lockKeyState, lockForeground, lockKeyHeld, lockSend
	t.Cleanup(func() { lockKeyState, lockForeground, lockKeyHeld, lockSend = get, foreground, held, send })
	lockKeyState = func(...uintptr) (uintptr, uintptr, error) { return 0, 0, nil }
	lockForeground = func() uintptr { return 10 }
	lockKeyHeld = func(uintptr) bool { return false }
	calls := 0
	lockSend = func(_ context.Context, events []input) error {
		calls++
		if calls == 1 {
			return errors.New("partial")
		}
		for i := range events {
			if (*keyboardInput)(unsafe.Pointer(&events[i].Mouse)).Flags&keyEventKeyUp == 0 {
				t.Fatal("uncertain toggle repeated")
			}
		}
		return nil
	}
	if err := SyncLocalLockState(context.Background(), 10, keyboardstate.State{Caps: "ON", Num: "ON", Hangul: "UNKNOWN"}); err == nil || calls != 2 {
		t.Fatal("partial injection claimed success")
	}
}
