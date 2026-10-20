//go:build windows

package launchrecord

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLiveDirectorySkipsOnlyConfirmedExitedSnapshotEntry(t *testing.T) {
	for _, tc := range []struct {
		name              string
		lookup            error
		present           bool
		probe             error
		skip, shouldProbe bool
	}{
		{"exited", windows.ERROR_INVALID_PARAMETER, false, nil, true, true},
		{"reused or still present", windows.ERROR_INVALID_PARAMETER, true, nil, false, true},
		{"snapshot failed", windows.ERROR_INVALID_PARAMETER, false, windows.ERROR_ACCESS_DENIED, false, true},
		{"access denied", windows.ERROR_ACCESS_DENIED, false, nil, false, false},
		{"unknown lookup error", windows.ERROR_GEN_FAILURE, false, nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probed := false
			path, err := liveProcessDirectory(123, func(uint32) (string, error) { return "", tc.lookup }, func(pid uint32) (bool, error) {
				probed = true
				if pid != 123 {
					t.Fatal("wrong process probed")
				}
				return tc.present, tc.probe
			})
			if (err == nil) != tc.skip || path != "" || probed != tc.shouldProbe {
				t.Fatal("unsafe process exclusion", path, err, probed)
			}
			if !tc.skip && !errors.Is(err, tc.lookup) {
				t.Fatal("lookup cause lost")
			}
		})
	}
	image := filepath.Join(t.TempDir(), "krasis.exe")
	path, err := liveProcessDirectory(123, func(uint32) (string, error) { return image, nil }, func(uint32) (bool, error) { t.Fatal("successful lookup retried"); return false, nil })
	if err != nil || path != filepath.Dir(image) {
		t.Fatal("live directory lost", path, err)
	}
	if _, err := liveProcessDirectory(123, func(uint32) (string, error) { return "", nil }, func(uint32) (bool, error) { t.Fatal("invalid path must not establish process exit"); return false, nil }); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty image path became safe ownership")
	}
}

func TestNativeLiveSnapshotPreservesCurrentProcess(t *testing.T) {
	present, err := liveProcessExists(uint32(os.Getpid()))
	if err != nil || !present {
		t.Fatal("current process missing", present, err)
	}
	path, err := liveProcessDirectory(uint32(os.Getpid()), processPath, liveProcessExists)
	self, e := os.Executable()
	if err != nil || e != nil || path != filepath.Dir(self) {
		t.Fatal("native process path lost", path, err, e)
	}
}
