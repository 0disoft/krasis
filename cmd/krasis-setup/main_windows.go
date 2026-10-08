//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/0disoft/krasis/internal/appinstall"
	"github.com/0disoft/krasis/internal/launchrecord"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func message(body string, confirm bool) bool {
	title, _ := windows.UTF16PtrFromString("Krasis")
	text, _ := windows.UTF16PtrFromString(body)
	flags := uintptr(0x40)
	if confirm {
		flags = 0x24
	}
	result, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), flags)
	return !confirm || result == 6
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }
func run(args []string, out io.Writer) int {
	action := "--install"
	quiet := false
	var parent uint64
	for _, arg := range args {
		if strings.HasPrefix(arg, "--parent-pid=") {
			var parseErr error
			parent, parseErr = strconv.ParseUint(strings.TrimPrefix(arg, "--parent-pid="), 10, 32)
			if parseErr != nil || parent == 0 {
				return 2
			}
		} else if arg == "--json" {
			quiet = true
		} else if action == "--install" && (arg == "--install" || arg == "--repair" || arg == "--remove" || arg == "--remove-worker" || arg == "--verify") {
			action = arg
		} else {
			return 2
		}
	}
	if parent != 0 && action != "--remove-worker" {
		return 2
	}
	data, err := payload()
	if err != nil {
		return result(out, quiet, "invalid_package", err)
	}
	p, err := appinstall.Read(data)
	if err != nil {
		return result(out, quiet, "invalid_package", err)
	}
	if action == "--verify" {
		return result(out, true, "verified", nil)
	}
	self, err := os.Executable()
	if err != nil {
		return result(out, quiet, "unavailable", err)
	}
	base := os.Getenv("LOCALAPPDATA")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if action == "--remove" {
		if !quiet && !message("Krasis 앱과 연결을 종료한 뒤 제거합니다. 기기 등록 정보·설정·진단은 보존됩니다. 보호 입력 제거에는 Windows 관리자 확인이 필요할 수 있습니다. 계속할까요?", true) {
			return result(out, quiet, "cancelled", nil)
		}
		// The uninstall worker runs outside the installed directory so the
		// registered uninstaller can itself be removed when its parent exits.
		if err = spawnRemoval(self, quiet); err != nil {
			return result(out, quiet, "remove_failed", err)
		}
		return result(out, quiet, "removal_requested", nil)
	}
	if action == "--remove-worker" {
		if parent != 0 {
			h, e := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(parent))
			if e == nil {
				status, e := windows.WaitForSingleObject(h, 10000)
				windows.CloseHandle(h)
				if e != nil || status != windows.WAIT_OBJECT_0 {
					return result(out, quiet, "remove_failed", appinstall.ErrBusy)
				}
			} else if !errors.Is(e, windows.ERROR_INVALID_PARAMETER) {
				return result(out, quiet, "remove_failed", e)
			}
		}
		err = appinstall.Remove(ctx, base)
		return result(out, quiet, "removed", err)
	}
	if !quiet && !message("이 사용자 계정에 Krasis를 설치하고 시작 메뉴에 등록합니다. 기존 기기 등록 정보와 설정은 유지됩니다. 계속할까요?", true) {
		return result(out, quiet, "cancelled", nil)
	}
	ui, err := appinstall.Install(base, self, p)
	if err != nil {
		return result(out, quiet, "install_failed", err)
	}
	if !quiet {
		if err = launchrecord.Start(ui); err != nil {
			return result(out, quiet, "installed_launch_failed", err)
		}
	}
	return result(out, quiet, "installed", nil)
}

func result(out io.Writer, quiet bool, state string, err error) int {
	code := 0
	if err != nil {
		code = 1
		if errors.Is(err, appinstall.ErrBusy) {
			state = "close_app_required"
		} else if errors.Is(err, appinstall.ErrOlder) || errors.Is(err, launchrecord.ErrOlder) {
			state = "newer_setup_required"
		} else if state == "removed" {
			state = "remove_failed"
		}
	}
	if quiet {
		if json.NewEncoder(out).Encode(struct {
			Schema string `json:"schema"`
			Code   int    `json:"code"`
			State  string `json:"state"`
		}{"krasis.setup-result/v1", code, state}) != nil {
			return 3
		}
	} else if state == "newer_setup_required" {
		message("설치본보다 새로운 Krasis가 있습니다. 최신 버전의 설치본으로 설치하거나 복구해 주세요.", false)
	} else if err != nil {
		message("처리를 완료하지 못했습니다. Krasis 앱·에이전트를 종료한 뒤 다시 시도해 주세요. 기존 설정과 진단은 보존됩니다.", false)
	} else if state == "removed" {
		message("Krasis를 제거했습니다. 기기 등록 정보·설정·진단은 보존했습니다.", false)
	}
	return code
}

func spawnRemoval(self string, quiet bool) error {
	dir, err := os.MkdirTemp("", "Krasis-uninstall-")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "krasis-setup.exe")
	if err = os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	check, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(data) != sha256.Sum256(check) {
		return appinstall.ErrInvalid
	}
	args := []string{"--remove-worker", fmt.Sprintf("--parent-pid=%d", os.Getpid())}
	if quiet {
		args = append(args, "--json")
	}
	command := exec.Command(path, args...)
	command.Dir = dir
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	// WMI/terminal independence is the same as ordinary Krasis launches.
	return launchrecord.StartIndependent(command)
}
