//go:build windows

package launcherui

import "unsafe"

func (a *productWindow) toggleUpdatePermission() {
	if a.busy || a.closing || a.pin.job != "" {
		return
	}
	peer := a.view.Fingerprints[a.selected()]
	if len(peer) != 64 {
		return
	}
	allowed := false
	for _, fp := range a.view.Update.Status.Allowed {
		if fp == peer {
			allowed = true
		}
	}
	action, text := "update_allow", "선택한 기기가 이 PC에 Krasis 개발 빌드를 보내고 자동 실행하도록 허용할까요?\n\n현재 개발 빌드는 배포자 서명이 없습니다. 이 기기를 업데이트 공급자로 신뢰할 때만 허용하세요. 적용 중 연결이 잠시 끊깁니다.\n\n허용은 이 페어링 기기에만 저장되며 같은 버튼에서 해제할 수 있습니다."
	if allowed {
		action, text = "update_deny", "선택한 기기의 자동 업데이트 권한을 해제할까요?"
	}
	a.busy = true
	answer, _, _ := user32.NewProc("MessageBoxW").Call(a.hwnd, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16("Krasis · 업데이트 권한"))), 0x134)
	a.busy = false
	if answer == 6 && !a.closing {
		a.performTarget(action, peer)
	}
}
