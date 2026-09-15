//go:build windows

package agentinstance

import (
	"errors"
	"strings"
	"syscall"
	"unsafe"
)

var (
	instanceKernel32 = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = instanceKernel32.NewProc("CreateMutexW")
)

func acquire(name string) (*Lease, error) {
	if !validName(name) {
		return nil, errors.New("agent instance name is invalid")
	}
	pointer, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, _, callErr := procCreateMutexW.Call(0, 1, uintptr(unsafe.Pointer(pointer)))
	if handle == 0 {
		return nil, callErr
	}
	if errors.Is(callErr, syscall.ERROR_ALREADY_EXISTS) {
		_ = syscall.CloseHandle(syscall.Handle(handle))
		return nil, ErrAlreadyRunning
	}
	return &Lease{close: func() error { return syscall.CloseHandle(syscall.Handle(handle)) }}, nil
}

func validName(name string) bool {
	return strings.HasPrefix(name, `Local\Krasis-Agent-Test-`) || name == productionName || name == controlName || name == `Local\Krasis-Control-Startup-v1` || name == `Local\Krasis-Log-agent-v1` || name == `Local\Krasis-Log-control-v1`
}
