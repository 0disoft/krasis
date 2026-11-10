//go:build linux

package securefs

import (
	"golang.org/x/sys/unix"
	"os"
)

func (directory *Directory) ReadNames() ([]string, error) {
	root, err := directory.pin()
	if err != nil {
		return nil, err
	}
	// Reopen "." relative to the pinned fd for an independent directory offset;
	// dup would share the offset and make repeated or concurrent lists incomplete.
	fd, err := linuxOpen(root, ".", unix.O_RDONLY|unix.O_DIRECTORY, 0, true)
	directory.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), directory.path)
	names, readErr := file.Readdirnames(-1)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	return names, closeErr
}
