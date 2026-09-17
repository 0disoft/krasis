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

// ControlRunning observes the legacy controller without acquiring its lease.
func ControlRunning() (bool, error) {
	return instanceRunning(controlName)
}

func instanceRunning(value string) (bool, error) {
	if !validName(value) {
		return false, errors.New("invalid instance name")
	}
	name, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return false, err
	}
	h, _, callErr := instanceKernel32.NewProc("OpenMutexW").Call(0x00100000, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		if errors.Is(callErr, syscall.ERROR_FILE_NOT_FOUND) {
			return false, nil
		}
		return false, callErr
	}
	return true, syscall.CloseHandle(syscall.Handle(h))
}
