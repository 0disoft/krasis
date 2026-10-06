//go:build windows

package launchrecord

import (
	"golang.org/x/sys/windows"
	"strings"
)

func UIProcessMatches(base string, pid uint32) bool {
	r, err := Inspect(base)
	if err != nil || pid == 0 {
		return false
	}
	path, err := processPath(pid)
	return err == nil && strings.EqualFold(path, r.Executable)
}
func processPath(pid uint32) (string, error) {
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(p)
	var path [32768]uint16
	n := uint32(len(path))
	if err = windows.QueryFullProcessImageName(p, 0, &path[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(path[:n]), nil
}
