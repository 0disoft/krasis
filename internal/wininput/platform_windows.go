//go:build windows

package wininput

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/0disoft/krasis/internal/lab"
)

const (
	smCMONITORS       = 80
	smCXVIRTUALSCREEN = 78
	smCYVIRTUALSCREEN = 79
	smXVIRTUALSCREEN  = 76
	smYVIRTUALSCREEN  = 77
	smCMOUSEBUTTONS   = 43
	smSWAPBUTTON      = 23
	tokenQuery        = 0x0008
	tokenElevation    = 20
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	procGetCurrentProcess    = kernel32.NewProc("GetCurrentProcess")
	procGetCurrentProcessID  = kernel32.NewProc("GetCurrentProcessId")
	procProcessIDToSessionID = kernel32.NewProc("ProcessIdToSessionId")
	procCloseHandle          = kernel32.NewProc("CloseHandle")
	procRtlGetVersion        = ntdll.NewProc("RtlGetVersion")
	procOpenProcessToken     = advapi32.NewProc("OpenProcessToken")
	procGetTokenInformation  = advapi32.NewProc("GetTokenInformation")
	procGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	procGetCursorPos         = user32.NewProc("GetCursorPos")
	procSetCursorPos         = user32.NewProc("SetCursorPos")
)

type Platform struct{}

func NewPlatform() *Platform {
	return &Platform{}
}

func (p *Platform) Diagnose(ctx context.Context) (lab.Diagnostics, error) {
	if err := ctx.Err(); err != nil {
		return lab.Diagnostics{}, err
	}

	windows, err := windowsVersion()
	if err != nil {
		return lab.Diagnostics{}, err
	}
	process, err := processInfo()
	if err != nil {
		return lab.Diagnostics{}, err
	}

	return lab.Diagnostics{
		SchemaVersion: lab.DiagnoseSchemaVersion,
		OS:            runtime.GOOS,
		Architecture:  runtime.GOARCH,
		Windows:       windows,
		Process:       process,
		Display: lab.DisplayInfo{
			MonitorCount: systemMetric(smCMONITORS),
			Left:         systemMetric(smXVIRTUALSCREEN),
			Top:          systemMetric(smYVIRTUALSCREEN),
			Width:        systemMetric(smCXVIRTUALSCREEN),
			Height:       systemMetric(smCYVIRTUALSCREEN),
			MouseButtons: systemMetric(smCMOUSEBUTTONS),
			SwapButtons:  systemMetric(smSWAPBUTTON) != 0,
		},
		Capabilities: lab.Capabilities{
			LowLevelKeyboardHook: user32.NewProc("SetWindowsHookExW").Find() == nil,
			LowLevelMouseHook:    user32.NewProc("SetWindowsHookExW").Find() == nil,
			RawInput: user32.NewProc("RegisterRawInputDevices").Find() == nil &&
				user32.NewProc("GetRawInputData").Find() == nil &&
				user32.NewProc("GetRawInputBuffer").Find() == nil,
			SendInput: user32.NewProc("SendInput").Find() == nil,
		},
	}, nil
}

type rtlOSVersionInfo struct {
	Size       uint32
	Major      uint32
	Minor      uint32
	Build      uint32
	PlatformID uint32
	CSDVersion [128]uint16
}

func windowsVersion() (lab.WindowsInfo, error) {
	info := rtlOSVersionInfo{Size: uint32(unsafe.Sizeof(rtlOSVersionInfo{}))}
	status, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	if status != 0 {
		return lab.WindowsInfo{}, fmt.Errorf("RtlGetVersion failed with NTSTATUS 0x%x", status)
	}
	return lab.WindowsInfo{Major: info.Major, Minor: info.Minor, Build: info.Build}, nil
}

func processInfo() (lab.ProcessInfo, error) {
	processID, _, _ := procGetCurrentProcessID.Call()
	var sessionID uint32
	result, _, callErr := procProcessIDToSessionID.Call(processID, uintptr(unsafe.Pointer(&sessionID)))
	if result == 0 {
		return lab.ProcessInfo{}, fmt.Errorf("ProcessIdToSessionId failed: %w", callErr)
	}

	process, _, _ := procGetCurrentProcess.Call()
	var token syscall.Handle
	result, _, callErr = procOpenProcessToken.Call(process, tokenQuery, uintptr(unsafe.Pointer(&token)))
	if result == 0 {
		return lab.ProcessInfo{}, fmt.Errorf("OpenProcessToken failed: %w", callErr)
	}
	defer procCloseHandle.Call(uintptr(token))

	var elevation uint32
	var returned uint32
	result, _, callErr = procGetTokenInformation.Call(
		uintptr(token),
		tokenElevation,
		uintptr(unsafe.Pointer(&elevation)),
		unsafe.Sizeof(elevation),
		uintptr(unsafe.Pointer(&returned)),
	)
	if result == 0 {
		return lab.ProcessInfo{}, fmt.Errorf("GetTokenInformation(TokenElevation) failed: %w", callErr)
	}

	return lab.ProcessInfo{SessionID: sessionID, Elevated: elevation != 0}, nil
}

func systemMetric(index uintptr) int32 {
	value, _, _ := procGetSystemMetrics.Call(index)
	return int32(value)
}
