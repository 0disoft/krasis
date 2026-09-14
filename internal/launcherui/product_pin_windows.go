//go:build windows

package launcherui

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/pinauth"
	"github.com/0disoft/krasis/internal/pindiscovery"
	"time"
	"unsafe"
)

const pinInvite = 2101
const pinSearch = 2102
const pinJoin = 2103
const pinCancel = 2104
const pinEvent = wmUIEvent + 1

type pinWindow struct {
	invite, search, join, cancel, input, candidates, status uintptr
	job                                                     string
	choices                                                 []pindiscovery.Candidate
	results                                                 chan pinResult
}

func addPINChoice(hwnd uintptr, text string) {
	send(hwnd, cbAddString, 0, uintptr(unsafe.Pointer(utf16(text))))
}

type pinResult struct {
	operation localapi.OperationResult
	choices   []pindiscovery.Candidate
	notice    string
	searched  bool
}

func (a *productWindow) createPIN(control func(string, string, uintptr, int32, int32, int32, int32, int) uintptr) {
	a.pin.results = make(chan pinResult, 1)
	control("STATIC", "새 기기 등록 · 왼쪽 PC에서 PIN 생성, 오른쪽 PC에서 검색 후 입력", 0, 20, 274, 555, 24, 0)
	control("STATIC", "등록하면 두 PC 간 키보드·마우스 제어를 허용합니다.", 0, 20, 301, 555, 24, 0)
	a.pin.invite = control("BUTTON", "PIN 만들기", wsTabStop, 20, 335, 170, 32, pinInvite)
	a.pin.search = control("BUTTON", "주변 기기 찾기", wsTabStop, 205, 335, 170, 32, pinSearch)
	a.pin.cancel = control("BUTTON", "등록 취소", wsTabStop, 390, 335, 175, 32, pinCancel)
	a.pin.candidates = control("COMBOBOX", "", wsTabStop|cbsDropDownList|wsVScroll, 20, 382, 285, 120, 0)
	a.pin.input = control("EDIT", "", wsTabStop|0x00800000|0x2000|0x20, 315, 382, 100, 30, 0)
	send(a.pin.input, 0x00C5, 6, 0) // EM_SETLIMITTEXT
	a.pin.join = control("BUTTON", "PIN으로 등록", wsTabStop, 425, 382, 140, 30, pinJoin)
	a.pin.status = control("STATIC", "PIN은 2분 동안만 유효합니다.", 0, 20, 428, 550, 58, 0)
}

func (a *productWindow) enablePIN() {
	for _, h := range a.rearmButtons {
		enable(h, !a.busy && !a.closing && a.view.State == "LOCAL" && a.pin.job == "" && len(a.view.Peers) > 0)
	}
	for _, h := range []uintptr{a.layoutLeft, a.layoutRight} {
		enable(h, !a.busy && !a.closing && a.view.State == "LOCAL" && a.pin.job == "" && len(a.view.Peers) > 0)
	}
	enable(a.exportButton, !a.busy && !a.closing && a.pin.job == "")
	idle := !a.busy && !a.closing && a.view.State == "LOCAL" && a.pin.job == ""
	for _, h := range []uintptr{a.pin.invite, a.pin.search, a.pin.input, a.pin.candidates, a.pin.join} {
		enable(h, idle)
	}
	enable(a.pin.cancel, !a.busy && !a.closing && a.pin.job != "")
}

func (a *productWindow) performPIN(action string) {
	if a.busy || a.closing {
		return
	}
	job := a.pin.job
	var choice pindiscovery.Candidate
	pin := ""
	if action == "join" {
		index := int32(send(a.pin.candidates, cbGetCurSel, 0, 0))
		if index < 0 || int(index) >= len(a.pin.choices) {
			setText(a.pin.status, "먼저 주변 기기를 찾아 선택하세요.")
			return
		}
		choice = a.pin.choices[index]
		pin = getText(a.pin.input)
		setText(a.pin.input, "")
		if !pinauth.Valid(pin) {
			setText(a.pin.status, "왼쪽 PC에 표시된 6자리 숫자를 입력하세요.")
			return
		}
	}
	a.busy = true
	a.enablePIN()
	for _, h := range []uintptr{a.peers, a.connect, a.disconnect, a.refresh, a.revoke} {
		enable(h, false)
	}
	hwnd := a.hwnd
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
		defer cancel()
		result := pinResult{}
		defer func() { a.pin.results <- result; procPostMessage.Call(hwnd, pinEvent, 0, 0) }()
		if action == "invite" || action == "join" {
			if err := a.startup.Ensure(ctx); err != nil {
				result.notice = "에이전트 준비 실패. 새로고침 후 다시 시도하세요."
				return
			}
		}
		client, err := localapi.OpenClient(a.control)
		if err != nil {
			result.notice = "에이전트에 접근할 수 없습니다."
			return
		}
		if action == "search" {
			result.choices, err = pindiscovery.Discover(ctx)
			result.searched = true
			if err != nil {
				result.notice = "기기 검색 실패. 네트워크와 방화벽 허용 상태를 확인하세요."
			}
			return
		}
		if action == "invite" || action == "join" {
			info, err := client.Runtime(ctx)
			if err != nil || info.Generation < 3 {
				result.notice = "양쪽 모두 새 에이전트가 필요합니다. 기존 연결을 해제하고 새로고침하세요."
				return
			}
		}
		allow := true
		request := localapi.OperationRequest{PairingID: job}
		switch action {
		case "invite":
			request = localapi.OperationRequest{PairingAction: "PIN_LISTEN", PeerAlias: "새 노트북", AllowInboundControl: &allow}
		case "join":
			request = localapi.OperationRequest{PairingAction: "PIN_DIAL", PeerAlias: "새 노트북", AllowInboundControl: &allow, Address: choice.Address, PairingID: choice.ID, ConfirmationCode: pin}
		case "cancel":
			request.PairingAction = "CANCEL"
		default:
			request.PairingAction = "STATUS"
		}
		result.operation, err = client.Pair(ctx, request)
		if err != nil {
			result.notice = "등록 요청을 확인하지 못했습니다. 잠시 후 다시 확인합니다."
			return
		}
		if result.operation.Code == "pairing_completed" && len(job) == 32 {
			connected, err := client.Connect(ctx, "새 노트북-"+job[:8])
			if err != nil || connected.Status != "COMPLETED" {
				result.notice = "등록 완료. 기기 목록에서 연결을 눌러주세요."
			} else {
				result.notice = "등록 완료. 저장된 배치로 연결을 시작했습니다."
			}
		}
	}()
}

func (a *productWindow) applyPIN(result pinResult) {
	if result.searched {
		a.pin.choices = result.choices
		send(a.pin.candidates, 0x014B, 0, 0)
		for _, choice := range result.choices {
			addPINChoice(a.pin.candidates, choice.Address)
		}
		if len(result.choices) > 0 {
			send(a.pin.candidates, cbSetCurSel, 0, 0)
			setText(a.pin.status, "상대를 선택하고 왼쪽 PC의 PIN을 입력하세요.")
		} else {
			setText(a.pin.status, "기기를 찾지 못했습니다. 왼쪽 PC에서 PIN을 만들고 다시 검색하세요.")
		}
	}
	op := result.operation
	if op.PairingID != "" {
		a.pin.job = op.PairingID
	}
	switch op.Code {
	case "pin_waiting":
		if op.Confirmation != "" {
			setText(a.pin.status, "PIN: "+op.Confirmation+" · 오른쪽 PC에 입력하세요. (최대 2분)")
		} else {
			setText(a.pin.status, "PIN을 확인하고 기기를 등록하고 있습니다.")
		}
	case "pairing_registering":
		setText(a.pin.status, "PIN 인증 완료. 기기 등록 중…")
	case "pairing_completed":
		a.pin.job = ""
		setText(a.pin.status, "등록 완료")
	case "pairing_failed", "pairing_expired", "pairing_cancelled", "pairing_finished", "pairing_not_found", "pairing_commit_uncertain":
		a.pin.job = ""
		setText(a.pin.status, "등록이 종료됐습니다. 양쪽 상태를 확인하세요. 다시 시도하려면 30초 후 새 PIN을 만드세요.")
	case "pairing_retry_later":
		setText(a.pin.status, "재시도 대기 중입니다. 30초 후 새 PIN으로 시작하세요.")
	default:
		if op.Code != "" && op.Status != "COMPLETED" {
			setText(a.pin.status, "등록 시작 실패. 연결을 해제하고 네트워크·방화벽을 확인하세요.")
		}
	}
	if result.notice != "" {
		setText(a.pin.status, result.notice)
	}
	a.enablePIN()
}
