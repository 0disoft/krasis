//go:build windows

package launchrecord

import (
	"errors"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
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
			path, e := liveProcessDirectory(entry.ProcessID, processPath, liveProcessExists)
			if e != nil {
				return nil, e
			}
			if path != "" {
				paths = append(paths, path)
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return paths, nil
}

// A Toolhelp entry is a snapshot, not evidence that its PID is still alive.
// Only a missing-PID lookup plus a fresh, complete snapshot can establish exit.
// Access denial, PID reuse and unreadable snapshots still block pruning.
func liveProcessDirectory(pid uint32, lookup func(uint32) (string, error), exists func(uint32) (bool, error)) (string, error) {
	path, err := lookup(pid)
	if err == nil {
		if !filepath.IsAbs(path) {
			return "", ErrInvalid
		}
		return filepath.Dir(path), nil
	}
	if !errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return "", err
	}
	present, probeErr := exists(pid)
	if probeErr != nil {
		return "", errors.Join(err, probeErr)
	}
	if !present {
		return "", nil
	}
	return "", err
}

func liveProcessExists(pid uint32) (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == pid {
			return true, nil
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, err
	}
	return false, nil
}
