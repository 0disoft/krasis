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
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(p)
	var path [32768]uint16
	n := uint32(len(path))
	if windows.QueryFullProcessImageName(p, 0, &path[0], &n) != nil {
		return false
	}
	return strings.EqualFold(windows.UTF16ToString(path[:n]), r.Executable)
}
