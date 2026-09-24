//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"github.com/0disoft/krasis/internal/wininput"
)

func runCredentialProbe() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	showCredentialProbeMessage("확인을 누른 뒤 2분 안에 B에서 Windows 보안 PIN 창을 띄우세요. PIN은 입력하지 마세요.", 0x40)
	result := wininput.RunCredentialUIProbe(ctx)
	fmt.Fprintln(os.Stdout, result)
	showCredentialProbeMessage("시험 결과: "+string(result)+"\n"+credentialProbeResultText(result), 0x40)
	if result == wininput.CredentialUIProbeDismissed {
		return 0
	}
	return 1
}

func credentialProbeResultText(result wininput.CredentialUIProbeResult) string {
	switch result {
	case wininput.CredentialUIProbeDismissed:
		return "Esc 입력 뒤 대상 창이 사라졌을 가능성이 있습니다. 실제로 닫혔는지 확인해 주세요."
	case wininput.CredentialUIProbeStillOpen:
		return "Esc를 보냈지만 대상 창이 계속 전면에 있었습니다."
	case wininput.CredentialUIProbeOtherForeground:
		return "시험 종료 때 다른 프로그램 창이 전면에 있었습니다. PIN 창이 떠 있었는지 확인해 주세요."
	case wininput.CredentialUIProbeIdentityUnknown:
		return "전면 창의 실행 프로그램을 확인할 수 없었습니다."
	case wininput.CredentialUIProbeDesktopUnavailable:
		return "입력 화면이 바뀌었거나 화면을 확인할 수 없었습니다."
	case wininput.CredentialUIProbeNoForeground:
		return "전면 창을 찾지 못했습니다."
	case wininput.CredentialUIProbeNotElevated:
		return "관리자 권한으로 실행되지 않았습니다."
	case wininput.CredentialUIProbeSendFailed:
		return "Esc 입력을 보내지 못했습니다."
	default:
		return "대상 PIN 창을 확인하지 못했습니다."
	}
}

func showCredentialProbeMessage(message string, style uintptr) {
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString("Krasis PIN 창 입력 시험")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), style,
	)
}
