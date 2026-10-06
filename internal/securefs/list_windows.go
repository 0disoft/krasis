//go:build windows

package securefs

import (
	"os"
	"syscall"
)

// ReadNames enumerates the pinned directory, including while its secure handle
// holds DELETE access. A path-based os.Open would omit delete sharing.
func (directory *Directory) ReadNames() ([]string, error) {
	if err := directory.ready("listing"); err != nil {
		return nil, err
	}
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}
	var handle syscall.Handle
	if err = syscall.DuplicateHandle(process, directory.handle, process, &handle, 0, false, syscall.DUPLICATE_SAME_ACCESS); err != nil {
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
