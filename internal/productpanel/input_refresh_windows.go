//go:build windows

package productpanel

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows"
)

func newInputRefresh(base, revision string) *inputRefresh {
	inspect := func() string {
		data, err := os.ReadFile(filepath.Join(base, "krasis-agent.exe"))
		if err != nil {
			return "UNKNOWN"
		}
		expected := sha256.Sum256(data)
		match, err := inputworker.InstallationMatchesAgent(expected)
		if err != nil {
			root, folderErr := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
			if folderErr == nil {
				if _, statErr := os.Lstat(filepath.Join(root, "Krasis", "Input")); errors.Is(statErr, os.ErrNotExist) {
					return "NOT_INSTALLED"
				}
			}
			return "UNKNOWN"
		}
		if match {
			launch, err := inputworker.OpenAgentLaunch(expected)
			if err == nil {
				if launch.Close() == nil {
					return "READY"
				}
			}
			// A stopped/unknown service on an otherwise matching installation is
			// a diagnostic condition, not permission for an automatic repair.
			return "UNKNOWN"
		}
		return "UPDATE_REQUIRED"
	}
	return &inputRefresh{inspect: inspect, run: func(ctx context.Context) error {
		return runInputInstaller(ctx, base, revision)
	}}
}

// Keep the directory and executable pinned against writes/replacement across
// the consent prompt. Checksums are integrity checks, not publisher signatures.
func pinRefreshFile(path string, directory bool) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, ErrInputRefreshFailed
	}
	flags, sharing := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT), uint32(windows.FILE_SHARE_READ)
	if directory {
		flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		sharing |= windows.FILE_SHARE_WRITE
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, sharing, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, ErrInputRefreshFailed
	}
	f := os.NewFile(uintptr(handle), "input-refresh-pin")
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory || !directory && info.NumberOfLinks != 1 {
		f.Close()
		return nil, ErrInputRefreshFailed
	}
	return f, nil
}

func runInputInstaller(ctx context.Context, base, revision string) error {
	if ctx.Err() != nil || len(revision) != 40 {
		return ErrInputRefreshFailed
	}
	guard, err := securefs.OpenReadDirectory(base)
	if err != nil {
		return ErrInputRefreshFailed
	}
	defer guard.Close()
	dir, err := pinRefreshFile(base, true)
	if err != nil {
		return err
	}
	defer dir.Close()
	manifest, err := guard.Open("SHA256SUMS.txt")
	if err != nil {
		return ErrInputRefreshFailed
	}
	data, err := io.ReadAll(io.LimitReader(manifest, 8193))
	manifest.Close()
	if err != nil || len(data) > 8192 {
		return ErrInputRefreshFailed
	}
	var pins []*os.File
	defer func() {
		for _, f := range pins {
			f.Close()
		}
	}()
	for _, name := range []string{"krasis-agent.exe", "krasis-install.exe"} {
		want, count := "", 0
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(strings.TrimSuffix(line, "\r"), "  ", 2)
			if len(parts) == 2 && parts[1] == name {
				want = parts[0]
				count++
			}
		}
		if count != 1 || len(want) != 64 {
			return ErrInputRefreshFailed
		}
		f, err := pinRefreshFile(filepath.Join(base, name), false)
		if err != nil {
			return err
		}
		pins = append(pins, f)
		info, err := f.Stat()
		if err != nil || info.Size() > 64<<20 {
			return ErrInputRefreshFailed
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, f); err != nil || hex.EncodeToString(hash.Sum(nil)) != want {
			return ErrInputRefreshFailed
		}
		build, err := buildinfo.Read(f)
		if err != nil || build.Path != "github.com/0disoft/krasis/cmd/"+strings.TrimSuffix(name, ".exe") {
			return ErrInputRefreshFailed
		}
		matched := false
		for _, setting := range build.Settings {
			if setting.Key == "vcs.revision" {
				matched = setting.Value == revision
			}
			if setting.Key == "vcs.modified" && setting.Value != "false" {
				return ErrInputRefreshFailed
			}
		}
		if !matched {
			return ErrInputRefreshFailed
		}
	}
	return elevateInputInstaller(ctx, filepath.Join(base, "krasis-install.exe"), base)
}

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            uintptr
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance, IDList                  uintptr
	Class                             *uint16
	ClassKey                          uintptr
	HotKey                            uint32
	Icon                              uintptr
	Process                           windows.Handle
}

func elevateInputInstaller(ctx context.Context, path, directory string) error {
	if ctx.Err() != nil {
		return ErrInputRefreshFailed
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && !errors.Is(err, syscall.Errno(1)) {
		return ErrInputRefreshFailed
	}
	defer windows.CoUninitialize()
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(path)
	args, _ := windows.UTF16PtrFromString("--install")
	dir, _ := windows.UTF16PtrFromString(directory)
	info := shellExecuteInfo{Mask: 0x40 | 0x100 | 0x400, Verb: verb, File: file, Parameters: args, Directory: dir, Show: windows.SW_SHOWNORMAL}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, callErr := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return ErrInputRefreshCancelled
		}
		return ErrInputRefreshFailed
	}
	if info.Process == 0 {
		return ErrInputRefreshPending
	}
	defer windows.CloseHandle(info.Process)
	// Once started, cancellation cannot justify racing an unfinished transaction.
	// Never kill the installer, delete its journal or start another instance.
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		result, err := windows.WaitForSingleObject(info.Process, 250)
		if err != nil {
			return ErrInputRefreshPending
		}
		if result == windows.WAIT_OBJECT_0 {
			var code uint32
			if windows.GetExitCodeProcess(info.Process, &code) != nil {
				return ErrInputRefreshPending
			}
			if code != 0 {
				return ErrInputRefreshFailed
			}
			return nil
		}
		if result != uint32(windows.WAIT_TIMEOUT) {
			return ErrInputRefreshPending
		}
	}
	return ErrInputRefreshPending
}
