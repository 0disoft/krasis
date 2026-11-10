//go:build linux

package launchrecord

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// LiveDirectories observes this user's product processes through procfs. An
// unreadable product image blocks cleanup; kill(pid, 0) alone cannot prove which
// executable owns a recycled PID.
func LiveDirectories() ([]string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	directories := map[string]bool{filepath.Dir(self): true}
	for _, entry := range entries {
		pid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil || pid == 0 {
			continue
		}
		root := filepath.Join("/proc", entry.Name())
		info, err := os.Stat(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, ErrInvalid
		}
		if stat.Uid != uint32(os.Geteuid()) {
			continue
		}
		path, err := os.Readlink(filepath.Join(root, "exe"))
		if err != nil {
			comm, commErr := os.ReadFile(filepath.Join(root, "comm"))
			if commErr != nil && !errors.Is(commErr, os.ErrNotExist) {
				return nil, commErr
			}
			if linuxProductImage(strings.TrimSpace(string(comm))) {
				return nil, err
			}
			continue
		}
		path = strings.TrimSuffix(path, " (deleted)")
		if linuxProductImage(filepath.Base(path)) {
			if !filepath.IsAbs(path) {
				return nil, ErrInvalid
			}
			directories[filepath.Dir(path)] = true
		}
	}
	paths := make([]string, 0, len(directories))
	for path := range directories {
		paths = append(paths, path)
	}
	return paths, nil
}

func linuxProductImage(name string) bool {
	switch name {
	case "krasis", "krasis-agent", "krasis-control", "krasis-cli", "krasis-install":
		return true
	}
	return false
}
