//go:build windows

package launcherui

import (
	"syscall"
	"unsafe"
)

const trayEvent = wmUIEvent + 3
const trayOpen = 2201
const trayDisconnect = 2202
const trayQuit = 2203

// NOTIFYICONDATAW, including the Vista+ trailing members.
type notifyIconData struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	BalloonIcon         uintptr
}

var notifyIcon = syscall.NewLazyDLL("shell32.dll").NewProc("Shell_NotifyIconW")

func trayLabel(state string) string {
	switch state {
	case "LOCAL":
		return "Krasis · 로컬"
	case "CONNECTED":
		return "Krasis · 연결됨"
	case "CONNECTING", "RECONNECTING":
		return "Krasis · 연결 중"
	case "DISCONNECTING":
		return "Krasis · 연결 해제 중"
	case "DEGRADED":
		return "Krasis · 연결 불안정"
	case "FAULTED":
		return "Krasis · 오류 — 창에서 확인"
	case "SAFE_LOCAL":
		return "Krasis · 안전 로컬 상태"
	default:
		return "Krasis · 상태 확인 필요"
	}
}

func (a *productWindow) trayData() notifyIconData {
	d := notifyIconData{Window: a.hwnd, ID: 1, Flags: 1 | 2 | 4 | 0x80, Callback: trayEvent}
	d.Size = uint32(unsafe.Sizeof(d))
	d.Icon, _, _ = user32.NewProc("LoadIconW").Call(0, 32512)
	text, _ := syscall.UTF16FromString(trayLabel(a.view.State))
	copy(d.Tip[:], text)
	return d
}

func (a *productWindow) installTray() {
	d := a.trayData()
	ok, _, _ := notifyIcon.Call(0, uintptr(unsafe.Pointer(&d)))
	a.trayReady = ok != 0
	if a.trayReady {
		d.Version = 4
		ok, _, _ = notifyIcon.Call(4, uintptr(unsafe.Pointer(&d)))
		if ok == 0 {
			a.removeTray()
		}
	}
	if !a.trayReady {
		a.showControl()
	}
}

func (a *productWindow) removeTray() {
	if a.trayReady {
		d := a.trayData()
		notifyIcon.Call(2, uintptr(unsafe.Pointer(&d)))
	}
	a.trayReady = false
}

func (a *productWindow) updateTray() {
	if !a.trayReady {
		return
	}
	d := a.trayData()
	ok, _, _ := notifyIcon.Call(1, uintptr(unsafe.Pointer(&d)))
	if ok == 0 {
		a.trayReady = false
		a.showControl()
	}
}

func (a *productWindow) showControl() {
	procShowWindow.Call(a.hwnd, 9) // SW_RESTORE
	user32.NewProc("SetForegroundWindow").Call(a.hwnd)
}

func (a *productWindow) trayMenu() {
	if a.trayMenuOpen || a.closing {
		return
	}
	menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		a.showControl()
		return
	}
	defer user32.NewProc("DestroyMenu").Call(menu)
	add := func(id uintptr, label string, disabled bool) {
		flags := uintptr(0)
		if disabled {
			flags = 1
		}
		user32.NewProc("AppendMenuW").Call(menu, flags, id, uintptr(unsafe.Pointer(utf16(label))))
	}
	add(trayOpen, "연결 관리 열기", false)
	add(trayDisconnect, "연결 해제", a.busy || a.view.State == "LOCAL")
	add(trayQuit, "연결 유지하고 관리 앱 종료", a.busy)
	var point struct{ X, Y int32 }
	user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&point)))
	user32.NewProc("SetForegroundWindow").Call(a.hwnd)
	a.trayMenuOpen = true
	command, _, _ := user32.NewProc("TrackPopupMenu").Call(menu, 0x100|2, uintptr(point.X), uintptr(point.Y), 0, a.hwnd, 0)
	a.trayMenuOpen = false
	procPostMessage.Call(a.hwnd, 0, 0, 0)
	if a.closing {
		return
	}
	switch command {
	case trayOpen:
		a.showControl()
	case trayDisconnect:
		if !a.busy {
			a.showControl()
			a.perform("disconnect")
		}
	case trayQuit:
		if !a.busy {
			a.quitRequested = true
			procPostMessage.Call(a.hwnd, wmClose, 0, 0)
		}
	}
}
