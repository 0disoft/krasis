//go:build windows

package launcherui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/launcher"
)

const (
	windowClassName = "KrasisLauncherWindow"
	windowTitle     = "Krasis 실기 테스트"

	wmCreate  = 0x0001
	wmDestroy = 0x0002
	wmClose   = 0x0010
	wmCommand = 0x0111
	wmApp     = 0x8000
	wmUIEvent = wmApp + 1

	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	wsChild            = 0x40000000
	wsBorder           = 0x00800000
	wsVScroll          = 0x00200000
	wsTabStop          = 0x00010000
	wsExClientEdge     = 0x00000200

	esAutoHScroll = 0x0080
	esAutoVScroll = 0x0040
	esMultiline   = 0x0004
	esReadOnly    = 0x0800

	bsPushButton    = 0x00000000
	cbsDropDownList = 0x0003

	cbAddString  = 0x0143
	cbGetCurSel  = 0x0147
	cbSetCurSel  = 0x014E
	cbnSelChange = 1
	bnClicked    = 0
	emSetSel     = 0x00B1
	emReplaceSel = 0x00C2
	wmSetFont    = 0x0030

	swShow       = 5
	swShowNormal = 1

	idRole = 1001 + iota
	idAlias
	idPeer
	idPort
	idCredentials
	idReports
	idDuration
	idSave
	idPrepare
	idStart
	idStop
	idCopy
	idOpenReports
	idStatus
	idLog
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procGetMessage          = user32.NewProc("GetMessageW")
	procGetWindowText       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSendMessage         = user32.NewProc("SendMessageW")
	procSetWindowText       = user32.NewProc("SetWindowTextW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procShellExecute        = shell32.NewProc("ShellExecuteW")
	globalApp               *application
)

type point struct {
	x int32
	y int32
}

type message struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   point
	private uint32
}

type windowClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSmall  uintptr
}

type uiEvent struct {
	kind   string
	text   string
	result launcher.OperationResult
}

type application struct {
	hwnd        uintptr
	role        uintptr
	alias       uintptr
	peer        uintptr
	port        uintptr
	credentials uintptr
	reports     uintptr
	duration    uintptr
	save        uintptr
	prepare     uintptr
	start       uintptr
	stop        uintptr
	copy        uintptr
	openReports uintptr
	status      uintptr
	log         uintptr

	base       string
	configPath string
	manager    launcher.Manager
	lastPlan   *launcher.SessionPlan
	pairPlan   *launcher.CredentialPlan
	closing    bool

	eventsMu sync.Mutex
	events   []uiEvent
}

func Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	base := filepath.Dir(executable)
	app := &application{
		base:       base,
		configPath: filepath.Join(base, "launcher-config.json"),
	}
	globalApp = app
	defer func() { globalApp = nil }()

	instance, _, _ := procGetModuleHandle.Call(0)
	icons := loadWindowIcons(instance)
	defer icons.close()
	className := utf16(windowClassName)
	cursor, _, _ := procLoadCursor.Call(0, 32512)
	class := windowClassEx{
		size:       uint32(unsafe.Sizeof(windowClassEx{})),
		wndProc:    syscall.NewCallback(windowProc),
		instance:   instance,
		icon:       icons.large.handle,
		iconSmall:  icons.small.handle,
		cursor:     cursor,
		background: 6,
		className:  className,
	}
	if result, _, callErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("register launcher window: %w", callErr)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), instance)
	hwnd, _, callErr := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16(windowTitle))),
		wsOverlappedWindow|wsVisible,
		0x80000000, 0x80000000, 780, 690,
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("create launcher window: %w", callErr)
	}
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	app.hwnd = hwnd
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	var msg message
	for {
		result, _, callErr := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("read launcher message: %w", callErr)
		}
		if result == 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func windowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	app := globalApp
	switch msg {
	case wmCreate:
		if app != nil {
			app.hwnd = hwnd
			if err := app.createControls(); err != nil {
				app.enqueue(uiEvent{kind: "fatal", text: err.Error()})
				return ^uintptr(0)
			}
		}
		return 0
	case wmCommand:
		if app != nil {
			app.handleCommand(int(wParam&0xffff), int((wParam>>16)&0xffff))
		}
		return 0
	case wmUIEvent:
		if app != nil {
			app.drainEvents()
		}
		return 0
	case wmClose:
		if app != nil && app.manager.Running() {
			app.closing = true
			setText(app.status, "안전하게 중지하는 중…")
			if err := app.manager.RequestStop(); err != nil {
				app.closing = false
				setText(app.status, "중지 실패: "+err.Error())
			}
			return 0
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func (app *application) createControls() error {
	font := stockFont()
	label := func(text string, x, y, width int32) {
		control := createControl(0, "STATIC", text, wsChild|wsVisible, x, y, width, 22, app.hwnd, 0)
		setFont(control, font)
	}
	edit := func(id int, x, y, width int32) uintptr {
		control := createControl(wsExClientEdge, "EDIT", "", wsChild|wsVisible|wsTabStop|esAutoHScroll, x, y, width, 25, app.hwnd, id)
		setFont(control, font)
		return control
	}
	button := func(id int, text string, x, y, width int32) uintptr {
		control := createControl(0, "BUTTON", text, wsChild|wsVisible|wsTabStop|bsPushButton, x, y, width, 30, app.hwnd, id)
		setFont(control, font)
		return control
	}

	label("역할", 20, 20, 90)
	app.role = createControl(wsExClientEdge, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|cbsDropDownList, 120, 18, 170, 120, app.hwnd, idRole)
	setFont(app.role, font)
	send(app.role, cbAddString, 0, uintptr(unsafe.Pointer(utf16("HP15 · Listener"))))
	send(app.role, cbAddString, 0, uintptr(unsafe.Pointer(utf16("gram13 · Connector"))))

	label("기기 별칭", 320, 20, 90)
	app.alias = edit(idAlias, 410, 18, 120)
	label("포트", 550, 20, 50)
	app.port = edit(idPort, 600, 18, 130)

	label("HP15 IP", 20, 62, 90)
	app.peer = edit(idPeer, 120, 60, 170)
	label("실행 시간", 320, 62, 90)
	app.duration = edit(idDuration, 410, 60, 120)

	label("자격 증명", 20, 104, 90)
	app.credentials = edit(idCredentials, 120, 102, 610)
	label("보고서", 20, 146, 90)
	app.reports = edit(idReports, 120, 144, 610)

	app.save = button(idSave, "설정 저장", 20, 190, 105)
	app.prepare = button(idPrepare, "Pair 준비", 135, 190, 105)
	app.start = button(idStart, "연결 시작", 250, 190, 105)
	app.stop = button(idStop, "안전 중지", 365, 190, 105)
	app.copy = button(idCopy, "보고서 복사", 480, 190, 115)
	app.openReports = button(idOpenReports, "폴더 열기", 605, 190, 125)

	app.status = createControl(0, "STATIC", "준비 중…", wsChild|wsVisible, 20, 235, 710, 25, app.hwnd, idStatus)
	setFont(app.status, font)
	app.log = createControl(wsExClientEdge, "EDIT", "", wsChild|wsVisible|wsBorder|wsVScroll|esMultiline|esAutoVScroll|esReadOnly, 20, 270, 710, 360, app.hwnd, idLog)
	setFont(app.log, font)
	enable(app.stop, false)
	enable(app.copy, false)

	if err := launcher.VerifyKit(app.base); err != nil {
		enable(app.save, false)
		enable(app.prepare, false)
		enable(app.start, false)
		setText(app.status, "패키지 검증 실패 · 실행 차단")
		appendLog(app.log, "패키지 파일이 손상됐거나 교체됨: "+err.Error()+"\r\n")
		return nil
	}
	appendLog(app.log, "패키지 파일 SHA-256 검증 완료\r\n")
	config := launcher.DefaultConfig(hostnameOrEmpty())
	if loaded, err := launcher.LoadConfig(app.configPath); err == nil {
		config = loaded
	} else if !errors.Is(err, os.ErrNotExist) {
		appendLog(app.log, "설정 파일을 읽지 못해 기본값을 사용함: "+err.Error()+"\r\n")
	}
	app.applyConfig(config)
	setText(app.status, "준비됨")
	return nil
}

func (app *application) handleCommand(id, notification int) {
	if id == idRole && notification == cbnSelChange {
		app.applyRoleDefaults()
		return
	}
	if notification != bnClicked {
		return
	}
	switch id {
	case idSave:
		if _, err := app.saveConfigFromControls(); err != nil {
			setText(app.status, "설정 오류: "+err.Error())
			return
		}
		setText(app.status, "설정 저장됨")
	case idPrepare:
		app.prepareCredentials()
	case idStart:
		app.startSession()
	case idStop:
		if err := app.manager.RequestStop(); err != nil {
			setText(app.status, "중지 실패: "+err.Error())
		} else {
			setText(app.status, "안전하게 중지하는 중…")
			enable(app.stop, false)
		}
	case idCopy:
		app.copyReports()
	case idOpenReports:
		app.openReportsDirectory()
	}
}

func (app *application) prepareCredentials() {
	config, err := app.saveConfigFromControls()
	if err != nil {
		setText(app.status, "설정 오류: "+err.Error())
		return
	}
	plan, err := config.BuildCredentialPlan(app.base, time.Now())
	if err != nil {
		setText(app.status, "준비 실패: "+err.Error())
		return
	}
	if err := launcher.PrepareCredentialPlan(plan); err != nil {
		setText(app.status, "준비 실패: "+err.Error())
		return
	}
	app.pairPlan = &plan
	app.setRunning(true, false)
	setText(app.status, "자격 증명 준비 중…")
	err = app.manager.Start(plan.Executable, plan.WorkingDir, plan.Commands, "", app.postLog, func(result launcher.OperationResult) {
		app.enqueue(uiEvent{kind: "pair-complete", result: result})
	})
	if err != nil {
		app.pairPlan = nil
		app.setRunning(false, false)
		setText(app.status, "준비 실패: "+err.Error())
	}
}

func (app *application) startSession() {
	config, err := app.saveConfigFromControls()
	if err != nil {
		setText(app.status, "설정 오류: "+err.Error())
		return
	}
	plan, err := config.BuildSessionPlan(app.base, time.Now())
	if err != nil {
		setText(app.status, "시작 실패: "+err.Error())
		return
	}
	if err := os.MkdirAll(filepath.Dir(plan.AgentReport), 0o700); err != nil {
		setText(app.status, "보고서 폴더 생성 실패: "+err.Error())
		return
	}
	app.lastPlan = &plan
	enable(app.copy, false)
	app.setRunning(true, true)
	setText(app.status, "연결 실행 중")
	err = app.manager.Start(plan.Executable, plan.WorkingDir, plan.Commands, plan.StopFile, app.postLog, func(result launcher.OperationResult) {
		app.enqueue(uiEvent{kind: "session-complete", result: result})
	})
	if err != nil {
		app.lastPlan = nil
		app.setRunning(false, false)
		setText(app.status, "시작 실패: "+err.Error())
	}
}

func (app *application) copyReports() {
	if app.lastPlan == nil {
		setText(app.status, "이번 실행의 보고서가 없음")
		return
	}
	payload, err := launcher.BundleReports(app.lastPlan.AgentReport, app.lastPlan.GuardReport)
	if err != nil {
		setText(app.status, "보고서 읽기 실패: "+err.Error())
		return
	}
	if err := writeClipboardText(app.hwnd, string(payload)); err != nil {
		setText(app.status, "클립보드 복사 실패: "+err.Error())
		return
	}
	setText(app.status, "이번 실행의 agent + guard 보고서를 복사함")
}

func (app *application) openReportsDirectory() {
	config, err := app.configFromControls()
	if err != nil {
		setText(app.status, "설정 오류: "+err.Error())
		return
	}
	directory := filepath.Join(app.base, filepath.Clean(config.ReportsDirectory))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		setText(app.status, "보고서 폴더 생성 실패: "+err.Error())
		return
	}
	if err := revealPath(directory); err != nil {
		setText(app.status, "폴더 열기 실패: "+err.Error())
	}
}

func (app *application) saveConfigFromControls() (launcher.Config, error) {
	config, err := app.configFromControls()
	if err != nil {
		return launcher.Config{}, err
	}
	if err := launcher.SaveConfig(app.configPath, config); err != nil {
		return launcher.Config{}, err
	}
	return config, nil
}

func (app *application) configFromControls() (launcher.Config, error) {
	selection := int(send(app.role, cbGetCurSel, 0, 0))
	role, edge := launcher.RoleConnect, "LEFT"
	if selection == 0 {
		role, edge = launcher.RoleListen, "RIGHT"
	}
	port, err := strconv.Atoi(strings.TrimSpace(getText(app.port)))
	if err != nil {
		return launcher.Config{}, errors.New("포트는 숫자여야 함")
	}
	config := launcher.Config{
		SchemaVersion:       launcher.ConfigSchemaVersion,
		Role:                role,
		DeviceAlias:         strings.TrimSpace(getText(app.alias)),
		PeerHost:            strings.TrimSpace(getText(app.peer)),
		Port:                port,
		Edge:                edge,
		CredentialDirectory: strings.TrimSpace(getText(app.credentials)),
		ReportsDirectory:    strings.TrimSpace(getText(app.reports)),
		Duration:            strings.TrimSpace(getText(app.duration)),
	}
	if err := config.Validate(); err != nil {
		return launcher.Config{}, err
	}
	return config, nil
}

func (app *application) applyConfig(config launcher.Config) {
	selection := uintptr(1)
	if config.Role == launcher.RoleListen {
		selection = 0
	}
	send(app.role, cbSetCurSel, selection, 0)
	setText(app.alias, config.DeviceAlias)
	setText(app.peer, config.PeerHost)
	setText(app.port, strconv.Itoa(config.Port))
	setText(app.credentials, config.CredentialDirectory)
	setText(app.reports, config.ReportsDirectory)
	setText(app.duration, config.Duration)
	app.updatePrepareLabel()
}

func (app *application) applyRoleDefaults() {
	if send(app.role, cbGetCurSel, 0, 0) == 0 {
		setText(app.alias, "S-A")
		setText(app.credentials, filepath.Join("secrets", "listener"))
	} else {
		setText(app.alias, "S-B")
		setText(app.credentials, filepath.Join("secrets", "connector"))
	}
	app.updatePrepareLabel()
}

func (app *application) updatePrepareLabel() {
	if send(app.role, cbGetCurSel, 0, 0) == 0 {
		setText(app.prepare, "새 Pair 생성")
	} else {
		setText(app.prepare, "Connector 준비")
	}
}

func (app *application) setRunning(running, stoppable bool) {
	enable(app.save, !running)
	enable(app.prepare, !running)
	enable(app.start, !running)
	enable(app.stop, running && stoppable)
}

func (app *application) postLog(text string) {
	app.enqueue(uiEvent{kind: "log", text: text})
}

func (app *application) enqueue(event uiEvent) {
	app.eventsMu.Lock()
	app.events = append(app.events, event)
	app.eventsMu.Unlock()
	if app.hwnd != 0 {
		procPostMessage.Call(app.hwnd, wmUIEvent, 0, 0)
	}
}

func (app *application) drainEvents() {
	app.eventsMu.Lock()
	events := app.events
	app.events = nil
	app.eventsMu.Unlock()
	for _, event := range events {
		switch event.kind {
		case "log":
			appendLog(app.log, event.text)
		case "pair-complete":
			app.setRunning(false, false)
			if event.result.Err != nil {
				setText(app.status, fmt.Sprintf("자격 증명 준비 실패 (%s, exit %d)", event.result.Command, event.result.ExitCode))
				app.pairPlan = nil
				continue
			}
			if app.pairPlan != nil {
				setText(app.credentials, app.pairPlan.CredentialDirectory)
				if _, err := app.saveConfigFromControls(); err != nil {
					setText(app.status, "준비는 끝났지만 설정 저장 실패: "+err.Error())
				} else {
					setText(app.status, "자격 증명 준비 완료")
				}
				if app.pairPlan.RevealDirectory != "" {
					_ = revealPath(app.pairPlan.RevealDirectory)
				}
			}
			app.pairPlan = nil
		case "session-complete":
			app.setRunning(false, false)
			if app.lastPlan != nil {
				_, agentErr := os.Stat(app.lastPlan.AgentReport)
				_, guardErr := os.Stat(app.lastPlan.GuardReport)
				enable(app.copy, agentErr == nil && guardErr == nil)
			}
			if event.result.Err != nil {
				setText(app.status, fmt.Sprintf("실행 종료 (%s, exit %d) · 보고서 확인", event.result.Command, event.result.ExitCode))
			} else {
				setText(app.status, "실행 완료 · 보고서 복사 가능")
			}
			if app.closing {
				procDestroyWindow.Call(app.hwnd)
			}
		case "fatal":
			setText(app.status, event.text)
		}
	}
}

func createControl(exStyle uintptr, class, text string, style uintptr, x, y, width, height int32, parent uintptr, id int) uintptr {
	hwnd, _, _ := procCreateWindowEx.Call(
		exStyle,
		uintptr(unsafe.Pointer(utf16(class))),
		uintptr(unsafe.Pointer(utf16(text))),
		style,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		parent, uintptr(id), 0, 0,
	)
	return hwnd
}

func utf16(value string) *uint16 {
	pointer, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		panic(err)
	}
	return pointer
}

func send(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	result, _, _ := procSendMessage.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func setText(hwnd uintptr, value string) {
	procSetWindowText.Call(hwnd, uintptr(unsafe.Pointer(utf16(value))))
}

func getText(hwnd uintptr) string {
	length, _, _ := procGetWindowTextLength.Call(hwnd)
	buffer := make([]uint16, length+1)
	procGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), length+1)
	return syscall.UTF16ToString(buffer)
}

func appendLog(hwnd uintptr, value string) {
	const maxLogCharacters = 64 << 10
	current := getText(hwnd)
	if len(current)+len(value) > maxLogCharacters {
		setText(hwnd, "[이전 로그 생략]\r\n")
	}
	send(hwnd, emSetSel, ^uintptr(0), ^uintptr(0))
	send(hwnd, emReplaceSel, 0, uintptr(unsafe.Pointer(utf16(value))))
}

func enable(hwnd uintptr, enabled bool) {
	value := uintptr(0)
	if enabled {
		value = 1
	}
	procEnableWindow.Call(hwnd, value)
}

func setFont(hwnd, font uintptr) {
	send(hwnd, wmSetFont, font, 1)
}

func stockFont() uintptr {
	proc := syscall.NewLazyDLL("gdi32.dll").NewProc("GetStockObject")
	font, _, _ := proc.Call(17)
	return font
}

func hostnameOrEmpty() string {
	hostname, _ := os.Hostname()
	return hostname
}

func revealPath(path string) error {
	result, _, callErr := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(utf16("open"))),
		uintptr(unsafe.Pointer(utf16(path))),
		0, 0, swShowNormal,
	)
	if result <= 32 {
		return fmt.Errorf("ShellExecuteW failed (%d): %w", result, callErr)
	}
	return nil
}
