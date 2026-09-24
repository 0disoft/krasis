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
	result := wininput.RunCredentialUIProbe(ctx)
	fmt.Fprintln(os.Stdout, result)
	message := "PIN 창 입력 시험 결과: " + string(result) + "\n실제 입력 가능 여부는 두 PC에서 추가로 확인해야 합니다."
	text, _ := syscall.UTF16PtrFromString(message)
	title, _ := syscall.UTF16PtrFromString("Krasis PIN 창 입력 시험")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x40,
	)
	if result == wininput.CredentialUIProbeDismissed {
		return 0
	}
	return 1
}
