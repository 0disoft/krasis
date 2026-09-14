//go:build windows

package launcherui

import (
	"context"
	"errors"
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/productpanel"
)

const productRefresh = 2001
const productConnect = 2002
const productDisconnect = 2003
const productPeers = 2004
const wmTimer = 0x0113

type productWindow struct {
	pin                                                       pinWindow
	log                                                       *productlog.Logger
	hwnd, peers, status, notice, connect, disconnect, refresh uintptr
	control                                                   string
	view                                                      productpanel.View
	busy, closing                                             bool
	ctx                                                       context.Context
	cancel                                                    context.CancelFunc
	result                                                    chan productpanel.View
	startup                                                   productpanel.Startup
}

var productApp *productWindow

func RunProductControl() (resultErr error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	log, _ := productlog.Open(os.Getenv("LOCALAPPDATA"), "control")
	log.Event("process_started")
	defer func() {
		if resultErr != nil {
			log.Event("process_failed")
		} else {
			log.Event("process_stopped")
		}
		log.Close()
	}()
	paths, err := productidentity.ResolveWindowsPaths(os.Getenv("LOCALAPPDATA"))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app := &productWindow{control: paths.Control, ctx: ctx, cancel: cancel, result: make(chan productpanel.View, 1)}
	app.log = log
	app.startup, err = productpanel.NewStartup(paths.Control)
	if err != nil {
		return err
	}
	productApp = app
	defer func() { productApp = nil }()
	instance, _, _ := procGetModuleHandle.Call(0)
	cursor, _, _ := procLoadCursor.Call(0, 32512)
	name := utf16("KrasisProductControlWindow")
	class := windowClassEx{size: uint32(unsafe.Sizeof(windowClassEx{})), wndProc: syscall.NewCallback(productWindowProc), instance: instance, cursor: cursor, background: 6, className: name}
	if value, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); value == 0 {
		return errors.New("register control window")
	}
	hwnd, _, _ := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(utf16("Krasis · 연결 관리"))), (wsOverlappedWindow&^(0x00040000|0x00010000))|wsVisible, 0x80000000, 0x80000000, 600, 550, 0, 0, instance, 0)
	if hwnd == 0 {
		return errors.New("create control window")
	}
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	app.perform("prepare")
	user32.NewProc("SetTimer").Call(hwnd, 1, 2000, 0)
	var msg message
	for {
		value, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(value) == -1 {
			return errors.New("read control message")
		}
		if value == 0 {
			return nil
		}
		if handled, _, _ := user32.NewProc("IsDialogMessageW").Call(hwnd, uintptr(unsafe.Pointer(&msg))); handled != 0 {
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func productWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := productApp
	if a == nil {
		value, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
		return value
	}
	switch msg {
	case wmCreate:
		a.hwnd = hwnd
		if !a.create() {
			return ^uintptr(0)
		}
		return 0
	case wmCommand:
		if int((wParam>>16)&0xffff) == bnClicked {
			switch int(wParam & 0xffff) {
			case pinInvite:
				a.performPIN("invite")
			case pinSearch:
				a.performPIN("search")
			case pinJoin:
				a.performPIN("join")
			case pinCancel:
				a.performPIN("cancel")
			case productRefresh:
				a.perform("prepare")
			case productConnect:
				a.perform("connect")
			case productDisconnect:
				a.perform("disconnect")
			}
		}
		return 0
	case wmTimer:
		if !a.busy && !a.closing {
			if a.pin.job != "" {
				a.performPIN("status")
			} else {
				a.perform("refresh")
			}
		}
		return 0
	case pinEvent:
		select {
		case result := <-a.pin.results:
			a.busy = false
			if a.closing {
				a.cancel()
				procDestroyWindow.Call(hwnd)
			} else {
				a.applyPIN(result)
				a.perform("refresh")
			}
		default:
		}
		return 0
	case wmUIEvent:
		select {
		case view := <-a.result:
			a.busy = false
			if a.closing {
				procDestroyWindow.Call(hwnd)
			} else {
				a.apply(view)
			}
		default:
		}
		return 0
	case wmClose:
		if a.pin.job != "" && !a.busy {
			a.performPIN("cancel")
			a.closing = true
			return 0
		}
		a.closing = true
		a.cancel()
		if !a.busy {
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	value, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return value
}

func (a *productWindow) create() bool {
	font := stockFont()
	control := func(class, text string, style uintptr, x, y, w, h int32, id int) uintptr {
		hwnd := createControl(0, class, text, style|wsChild|wsVisible, x, y, w, h, a.hwnd, id)
		setFont(hwnd, font)
		return hwnd
	}
	control("STATIC", "이미 페어링한 기기를 선택해 연결하세요.", 0, 20, 18, 550, 25, 0)
	a.peers = control("COMBOBOX", "", wsTabStop|cbsDropDownList|wsVScroll, 20, 55, 545, 160, productPeers)
	a.connect = control("BUTTON", "연결", wsTabStop, 20, 100, 170, 35, productConnect)
	a.disconnect = control("BUTTON", "연결 해제", wsTabStop, 205, 100, 170, 35, productDisconnect)
	a.refresh = control("BUTTON", "새로고침", wsTabStop, 390, 100, 175, 35, productRefresh)
	a.status = control("STATIC", "상태 확인 중…", 0, 20, 150, 545, 22, 0)
	a.notice = control("STATIC", "", 0, 20, 177, 545, 38, 0)
	control("STATIC", "창을 닫아도 연결은 유지됩니다. 중지하려면 연결 해제를 누르세요.", 0, 20, 226, 550, 30, 0)
	a.createPIN(control)
	a.apply(productpanel.View{})
	return a.peers != 0 && a.connect != 0 && a.disconnect != 0 && a.refresh != 0 && a.status != 0 && a.notice != 0
}

func (a *productWindow) selected() string {
	index := int32(send(a.peers, cbGetCurSel, 0, 0))
	if index < 0 || int(index) >= len(a.view.Peers) {
		return ""
	}
	return a.view.Peers[index]
}

func (a *productWindow) perform(action string) {
	if a.busy || a.closing {
		return
	}
	alias := a.selected()
	a.busy = true
	a.enablePIN()
	for _, hwnd := range []uintptr{a.peers, a.connect, a.disconnect, a.refresh} {
		enable(hwnd, false)
	}
	hwnd := a.hwnd
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
		defer cancel()
		if action == "prepare" || action == "connect" {
			a.log.Event("prepare_started")
			if err := a.startup.Ensure(ctx); err != nil {
				a.log.Event("prepare_failed")
				view := productpanel.Execute(ctx, func() (productpanel.Client, error) { return localapi.OpenClient(a.control) }, "refresh", "")
				view.Notice = productpanel.StartupNotice(err)
				a.result <- view
				procPostMessage.Call(hwnd, wmUIEvent, 0, 0)
				return
			}
			a.log.Event("prepare_completed")
			if action == "prepare" {
				action = "refresh"
			}
		}
		view := productpanel.Execute(ctx, func() (productpanel.Client, error) { return localapi.OpenClient(a.control) }, action, alias)
		a.result <- view
		procPostMessage.Call(hwnd, wmUIEvent, 0, 0)
	}()
}

func (a *productWindow) apply(view productpanel.View) {
	selected := a.selected()
	a.view = view
	send(a.peers, 0x014B, 0, 0) // CB_RESETCONTENT
	index := 0
	for i, alias := range view.Peers {
		send(a.peers, cbAddString, 0, uintptr(unsafe.Pointer(utf16(alias))))
		if alias == selected {
			index = i
		}
	}
	if len(view.Peers) > 0 {
		send(a.peers, cbSetCurSel, uintptr(index), 0)
	}
	state := view.State
	if state == "" {
		state = "확인 필요"
	}
	setText(a.status, "상태: "+state)
	if view.Notice == "" && len(view.Peers) == 0 {
		view.Notice = "페어링된 기기가 없습니다. 아래에서 PIN으로 새 기기를 등록하세요."
	}
	if a.log == nil || a.log.Failed() {
		view.Notice = "로그 저장 불가 또는 기록 누락이 발생했습니다. " + view.Notice
	}
	setText(a.notice, view.Notice)
	enable(a.peers, len(view.Peers) > 0)
	enable(a.connect, view.State == "LOCAL" && len(view.Peers) > 0)
	enable(a.disconnect, view.State != "" && view.State != "LOCAL")
	enable(a.refresh, true)
	a.enablePIN()
}
