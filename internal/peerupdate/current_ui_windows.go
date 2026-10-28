//go:build windows

package peerupdate

import (
	"os"

	"golang.org/x/sys/windows"
)

// Prevent stage renames and executable/metadata writes through process creation.
// securefs separately rejects reparse points throughout the absolute path.
func pinCurrentUIPath(path string, directory bool) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags, sharing := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT), uint32(windows.FILE_SHARE_READ)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		sharing |= windows.FILE_SHARE_WRITE
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, sharing, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(handle), "current-ui-pin")
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		f.Close()
		return nil, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		f.Close()
		return nil, ErrDenied
	}
	return f, nil
}
