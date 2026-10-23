//go:build windows

package securefs

import (
	"os"
	"syscall"
)

// ReadNames enumerates the pinned directory, including while its secure handle
// holds DELETE access. A path-based os.Open would omit delete sharing.
func (directory *Directory) ReadNames() ([]string, error) {
	root, err := directory.pin()
	if err != nil {
		return nil, err
	}
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		directory.mu.RUnlock()
		return nil, err
	}
	var handle syscall.Handle
	err = syscall.DuplicateHandle(process, root, process, &handle, 0, false, syscall.DUPLICATE_SAME_ACCESS)
	directory.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(handle), directory.path)
	names, readErr := f.Readdirnames(-1)
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	return names, closeErr
}
