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
// unused. Only product UI/agent images in this session are relevant.
func LiveDirectories() ([]string, error) {
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		return nil, err
	}
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
		if name == "krasis.exe" || name == "krasis-agent.exe" || name == "krasis-control.exe" {
			var other uint32
			if e := windows.ProcessIdToSessionId(entry.ProcessID, &other); e != nil {
				return nil, e
			}
			if other == session {
				path, e := processPath(entry.ProcessID)
				if e != nil {
					return nil, e
				}
				paths = append(paths, filepath.Dir(path))
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return paths, nil
}
