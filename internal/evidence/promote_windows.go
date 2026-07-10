//go:build windows

package evidence

import (
	"errors"
	"os"
	"syscall"
)

const moveFileWriteThrough = 0x00000008

var procMoveFileExW = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func promoteNoReplace(source, destination string) error {
	sourcePointer, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPointer, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	result, _, callErr := procMoveFileExW.Call(
		uintptr(unsafePointer(sourcePointer)),
		uintptr(unsafePointer(destinationPointer)),
		moveFileWriteThrough,
	)
	if result != 0 {
		return nil
	}
	if errors.Is(callErr, syscall.ERROR_ALREADY_EXISTS) || errors.Is(callErr, syscall.ERROR_FILE_EXISTS) {
		return ErrOutputExists
	}
	if _, statErr := os.Lstat(destination); statErr == nil {
		return ErrOutputExists
	}
	return callErr
}
