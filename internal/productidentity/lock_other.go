//go:build !windows

package productidentity

import (
	"errors"
	"os"
	"syscall"
)

func acquireFileLock(file *os.File) (func() error, error) {
	return acquireIdentityLock(file, false)
}

func acquireSharedFileLock(file *os.File) (func() error, error) {
	return acquireIdentityLock(file, true)
}

func acquireIdentityLock(file *os.File, shared bool) (func() error, error) {
	flags := syscall.LOCK_EX | syscall.LOCK_NB
	if shared {
		flags = syscall.LOCK_SH | syscall.LOCK_NB
	}
	if err := syscall.Flock(int(file.Fd()), flags); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrStoreBusy
		}
		return nil, err
	}
	return func() error { return syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }, nil
}
