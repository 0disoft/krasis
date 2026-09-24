//go:build windows && desktop

package workspacehost

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/0disoft/krasis/internal/productpanel"
	"golang.org/x/sys/windows"
)

func credentialProbeAgentPath(controlExecutable string) (string, error) {
	if !filepath.IsAbs(controlExecutable) || filepath.Base(controlExecutable) != "krasis-control.exe" {
		return "", errors.New("invalid controller executable")
	}
	return filepath.Join(filepath.Dir(controlExecutable), "krasis-agent.exe"), nil
}

func launchCredentialProbe(control string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	agent, err := credentialProbeAgentPath(self)
	if err != nil {
		return err
	}
	startup, err := productpanel.NewStartup(control)
	if err != nil {
		return err
	}
	if err := startup.Verify(); err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	path, err := windows.UTF16PtrFromString(agent)
	if err != nil {
		return err
	}
	args, _ := windows.UTF16PtrFromString("credential-ui-probe")
	return windows.ShellExecute(0, verb, path, args, nil, windows.SW_SHOWNORMAL)
}

func showCredentialProbeLaunchError() {
	message, _ := syscall.UTF16PtrFromString("PIN 창 입력 시험을 시작하지 못했습니다. g60 파일과 관리자 권한 확인을 살펴보세요.")
	title, _ := syscall.UTF16PtrFromString("Krasis PIN 창 입력 시험")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(
		0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10,
	)
}
