//go:build windows

package launchrecord

import (
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"unsafe"
)

// Unknown process ownership blocks pruning instead of guessing that a folder is
// unused. Preserve product images in every Windows session, including installers.
func LiveDirectories() ([]string, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	var paths []string
	for err == nil {
		name := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		// The SYSTEM input service runs only from Program Files, never a stage.
		if name == "krasis.exe" || name == "krasis-agent.exe" || name == "krasis-control.exe" || name == "krasis-cli.exe" || name == "krasis-install.exe" {
			path, e := processPath(entry.ProcessID)
			if e != nil {
				return nil, e
			}
			paths = append(paths, filepath.Dir(path))
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return paths, nil
}
