//go:build windows

package evidence

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	outputFileAttributeReparsePoint = 0x00000400
	outputInvalidFileAttributes     = 0xffffffff
)

var (
	outputKernel32              = syscall.NewLazyDLL("kernel32.dll")
	outputGetFileAttributesProc = outputKernel32.NewProc("GetFileAttributesW")
	getOutputPathAttributes     = getWindowsOutputPathAttributes
)

func validateOutputDirectory(path string) error {
	volume := filepath.VolumeName(path)
	current := volume + string(os.PathSeparator)
	remainder := strings.TrimPrefix(path, current)
	for _, component := range strings.Split(remainder, string(os.PathSeparator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		attributes, err := getOutputPathAttributes(current)
		if err != nil {
			return fmt.Errorf("inspect evidence output path attributes: %w", err)
		}
		if attributes&outputFileAttributeReparsePoint != 0 {
			return fmt.Errorf("%w: reparse point at %s", ErrUnsafeOutputPath, current)
		}
	}
	return nil
}

func getWindowsOutputPathAttributes(path string) (uint32, error) {
	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	attributes, _, callErr := outputGetFileAttributesProc.Call(uintptr(unsafePointer(pointer)))
	if uint32(attributes) == outputInvalidFileAttributes {
		return 0, callErr
	}
	return uint32(attributes), nil
}
