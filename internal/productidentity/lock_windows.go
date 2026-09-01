//go:build windows

package productidentity

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(33)
)

var (
	identityKernel32 = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = identityKernel32.NewProc("LockFileEx")
	procUnlockFileEx = identityKernel32.NewProc("UnlockFileEx")
)

func acquireFileLock(file *os.File) (func() error, error) {
	var overlapped syscall.Overlapped
	result, _, callErr := procLockFileEx.Call(
		file.Fd(), lockfileFailImmediately|lockfileExclusiveLock, 0, 1, 0,
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if result == 0 {
		if errors.Is(callErr, errorLockViolation) {
			return nil, ErrStoreBusy
		}
		return nil, callErr
	}
	return func() error {
		result, _, callErr := procUnlockFileEx.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
		if result == 0 {
			return callErr
		}
		return nil
	}, nil
}
