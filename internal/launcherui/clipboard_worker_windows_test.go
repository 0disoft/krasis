//go:build windows

package launcherui

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"unsafe"
)

func TestSharedClipboardPumpResetsWithoutConsentOrTrust(t *testing.T) {
	for _, peer := range []string{"", "untrusted"} {
		p := clipboardPump{peer: "old", sequence: 42}
		// Invalid owner/control are deliberate: these cases must return before any
		// native clipboard access or local-agent request.
		p.sync(context.Background(), 0, "invalid-control", peer, nil)
		if p.peer != "" || p.sequence != 0 {
			t.Fatal(p)
		}
	}
}

func TestClipboardOwnerSurvivesPendingUpdateTitleWhileHidden(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class := utf16("STATIC")
	title := utf16("Krasis clipboard owner test")
	// A hidden, unfocused native window only; this test never touches clipboard
	// content, installs hooks, or sends physical input.
	hwnd, _, err := clipUser.NewProc("CreateWindowExW").Call(0,
		uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0,
		0, 0, 100, 100, 0, 0, 0, 0)
	runtime.KeepAlive(class)
	runtime.KeepAlive(title)
	if hwnd == 0 {
		t.Fatal("create hidden owner", err)
	}
	defer func() {
		if hwnd != 0 {
			clipUser.NewProc("DestroyWindow").Call(hwnd)
		}
	}()
	var owner atomic.Uintptr
	owner.Store(hwnd)
	if got := clipboardOwnerWindow(&owner); got != hwnd {
		t.Fatal("hidden owner was rejected", got)
	}
	pending := utf16("Krasis clipboard owner test · 새 버전 대기")
	updated, _, err := clipUser.NewProc("SetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(pending)))
	runtime.KeepAlive(pending)
	if updated == 0 {
		t.Fatal("set pending update title", err)
	}
	if got := clipboardOwnerWindow(&owner); got != hwnd {
		t.Fatal("title change lost the clipboard owner", got)
	}
	if visible, _, _ := clipUser.NewProc("IsWindowVisible").Call(hwnd); visible != 0 {
		t.Fatal("clipboard owner test unexpectedly showed a window")
	}
	if destroyed, _, err := clipUser.NewProc("DestroyWindow").Call(hwnd); destroyed == 0 {
		t.Fatal("destroy owner", err)
	}
	hwnd = 0
	if clipboardOwnerWindow(&owner) != 0 {
		t.Fatal("destroyed owner was accepted")
	}
}

func TestClipboardOwnerRejectsUnavailableAndForeignWindows(t *testing.T) {
	var owner atomic.Uintptr
	if clipboardOwnerWindow(nil) != 0 || clipboardOwnerWindow(&owner) != 0 {
		t.Fatal("missing owner was accepted")
	}
	desktop, _, _ := clipUser.NewProc("GetDesktopWindow").Call()
	if desktop == 0 {
		t.Fatal("desktop handle unavailable")
	}
	owner.Store(desktop)
	if clipboardOwnerWindow(&owner) != 0 {
		t.Fatal("another process's window was accepted")
	}
}
