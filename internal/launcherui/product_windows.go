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
const productRevoke = 2005
const productExport = 2006
const layoutLeft = 2301
const layoutRight = 2302
const exportEvent = wmUIEvent + 2
const wmTimer = 0x0113

type productWindow struct {
	layoutLeft, layoutRight                                   uintptr
	trayReady, trayMenuOpen, quitRequested                    bool
	taskbarCreated                                            uint32
	exportButton, exportPath                                  uintptr
	exportResults                                             chan string
	revoke                                                    uintptr
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
	hwnd, _, _ := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(utf16("Krasis · 연결 관리"))), (wsOverlappedWindow&^(0x00040000|0x00010000))|wsVisible, 0x80000000, 0x80000000, 600, 715, 0, 0, instance, 0)
	if hwnd == 0 {
		return errors.New("create control window")
	}
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)
	messageID, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(utf16("TaskbarCreated"))))
	app.taskbarCreated = uint32(messageID)
	app.installTray()
	defer app.removeTray()
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
	if a.taskbarCreated != 0 && msg == a.taskbarCreated && !a.closing {
		a.removeTray()
		a.installTray()
		return 0
	}
	switch msg {
	case trayEvent:
		if a.closing {
			return 0
		}
		switch uint32(lParam & 0xffff) {
		case 0x400, 0x401:
			a.showControl() // NIN_SELECT / NIN_KEYSELECT
		case 0x007B:
			a.trayMenu() // WM_CONTEXTMENU
		}
		return 0
	case wmCreate:
		a.hwnd = hwnd
		if !a.create() {
			return ^uintptr(0)
		}
		return 0
	case wmCommand:
		if int((wParam>>16)&0xffff) == bnClicked {
			switch int(wParam & 0xffff) {
			case layoutLeft:
				a.performTarget("layout_LEFT", a.view.Fingerprints[a.selected()])
			case layoutRight:
				a.performTarget("layout_RIGHT", a.view.Fingerprints[a.selected()])
			case productExport:
				a.exportDiagnostics()
			case productRevoke:
				a.revokePeer()
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
		if !a.busy && !a.closing && !a.trayMenuOpen {
			if a.pin.job != "" {
				a.performPIN("status")
			} else {
				a.perform("refresh")
			}
		}
		return 0
	case exportEvent:
		select {
		case result := <-a.exportResults:
			a.busy = false
			if a.closing {
				procDestroyWindow.Call(hwnd)
			} else {
				a.apply(a.view)
				setText(a.exportPath, result)
			}
		default:
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
		if a.trayMenuOpen {
			return 0
		}
		if !a.quitRequested && a.trayReady {
			if a.busy || a.pin.job != "" {
				setText(a.notice, "진행 중인 작업이나 기기 등록을 완료 또는 취소한 뒤 창을 닫으세요.")
			} else {
				procShowWindow.Call(hwnd, 0)
			}
			return 0
		}
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
		a.removeTray()
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
	control("STATIC", "창을 닫으면 트레이에 남습니다.\n중지하려면 연결 해제를 누르세요.", 0, 20, 222, 355, 46, 0)
	a.revoke = control("BUTTON", "기기 등록 해제", wsTabStop, 390, 226, 175, 30, productRevoke)
	a.createPIN(control)
	a.exportResults = make(chan string, 1)
	a.exportButton = control("BUTTON", "진단 내보내기", wsTabStop, 20, 490, 170, 30, productExport)
	control("STATIC", "최근 이벤트만 저장 · 이전 내보내기 파일 교체", 0, 205, 494, 360, 25, 0)
	a.exportPath = control("EDIT", "저장 후 이 칸의 파일 경로를 복사할 수 있습니다.", wsTabStop|0x0800|0x0080, 20, 532, 545, 26, 0)
	control("STATIC", "선택한 기기의 위치 · 연결 해제 후 양쪽 PC에서 각각 설정", 0, 20, 580, 545, 25, 0)
	a.layoutLeft = control("BUTTON", "상대가 왼쪽", wsTabStop, 20, 616, 260, 32, layoutLeft)
	a.layoutRight = control("BUTTON", "상대가 오른쪽", wsTabStop, 305, 616, 260, 32, layoutRight)
	a.apply(productpanel.View{})
	return a.peers != 0 && a.connect != 0 && a.disconnect != 0 && a.refresh != 0 && a.status != 0 && a.notice != 0 && a.exportButton != 0 && a.exportPath != 0
}

func (a *productWindow) exportDiagnostics() {
	if a.busy || a.closing || a.pin.job != "" {
		return
	}
	a.busy = true
	a.enablePIN()
	for _, h := range []uintptr{a.peers, a.connect, a.disconnect, a.refresh, a.revoke} {
		enable(h, false)
	}
	hwnd, base := a.hwnd, os.Getenv("LOCALAPPDATA")
	setText(a.exportPath, "진단 파일 저장 중…")
	go func() {
		path, err := productlog.Export(base)
		if err != nil {
			path = "저장 실패. 저장 공간과 Krasis 폴더 접근 권한을 확인하세요."
		}
		a.exportResults <- path
		procPostMessage.Call(hwnd, exportEvent, 0, 0)
	}()
}

func (a *productWindow) selected() string {
	index := int32(send(a.peers, cbGetCurSel, 0, 0))
	if index < 0 || int(index) >= len(a.view.Peers) {
		return ""
	}
	return a.view.Peers[index]
}

func (a *productWindow) perform(action string) {
	a.performTarget(action, a.selected())
}

func (a *productWindow) revokePeer() {
	if a.busy || a.closing || a.view.State != "LOCAL" || a.pin.job != "" {
		return
	}
	fingerprint := a.view.Fingerprints[a.selected()]
	if len(fingerprint) != 64 {
		return
	}
	a.busy = true
	answer, _, _ := user32.NewProc("MessageBoxW").Call(a.hwnd, uintptr(unsafe.Pointer(utf16("선택한 기기의 등록을 이 PC에서 해제할까요?\n다시 사용하려면 재등록해야 합니다. 상대 PC의 기록은 삭제하지 않습니다."))), uintptr(unsafe.Pointer(utf16("Krasis · 기기 등록 해제"))), 0x134)
	a.busy = false
	if a.closing {
		a.cancel()
		procDestroyWindow.Call(a.hwnd)
		return
	}
	if answer == 6 {
		a.performTarget("revoke", fingerprint)
	}
}

func (a *productWindow) performTarget(action, alias string) {
	if a.busy || a.closing {
		return
	}
	a.busy = true
	a.enablePIN()
	for _, hwnd := range []uintptr{a.peers, a.connect, a.disconnect, a.refresh, a.revoke} {
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
		open := func() (productpanel.Client, error) { return localapi.OpenClient(a.control) }
		var view productpanel.View
		if action == "layout_LEFT" || action == "layout_RIGHT" {
			view = productpanel.SetLayout(ctx, open, alias, action[len("layout_"):])
		} else if action == "revoke" {
			view = productpanel.Revoke(ctx, open, alias)
		} else {
			view = productpanel.Execute(ctx, open, action, alias)
		}
		a.result <- view
		procPostMessage.Call(hwnd, wmUIEvent, 0, 0)
	}()
}

func (a *productWindow) apply(view productpanel.View) {
	selected := a.selected()
	a.view = view
	a.updateTray()
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
	enable(a.revoke, view.State == "LOCAL" && len(view.Peers) > 0 && a.pin.job == "")
	a.enablePIN()
}
