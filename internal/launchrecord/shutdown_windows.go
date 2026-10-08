//go:build windows

package launchrecord

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// UIShutdown retains process handles, so a recycled PID cannot become a target.
// Only this user's current-session, byte-verified launch images are eligible.
type UIShutdown struct{ processes []uiShutdownProcess }
type uiShutdownProcess struct {
	pid    uint32
	handle windows.Handle
}

func PrepareUIShutdown(base string) (*UIShutdown, error) {
	plan := &UIShutdown{}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	var session uint32
	if err = windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err != nil {
		return nil, err
	}
	me, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "krasis.exe") {
			var targetSession uint32
			if e := windows.ProcessIdToSessionId(entry.ProcessID, &targetSession); e != nil {
				plan.Close()
				return nil, e
			}
			if targetSession == session {
				p, e := prepareShutdownProcess(base, entry.ProcessID, me.User.Sid)
				if e != nil {
					plan.Close()
					return nil, e
				}
				if p.handle != 0 {
					plan.processes = append(plan.processes, p)
				}
			}
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		plan.Close()
		return nil, err
	}
	return plan, nil
}

func prepareShutdownProcess(base string, pid uint32, owner *windows.SID) (uiShutdownProcess, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return uiShutdownProcess{}, err
	}
	keep := false
	defer func() {
		if !keep {
			windows.CloseHandle(h)
		}
	}()
	if state, e := windows.WaitForSingleObject(h, 0); e != nil {
		return uiShutdownProcess{}, e
	} else if state == windows.WAIT_OBJECT_0 {
		return uiShutdownProcess{}, nil
	}
	var token windows.Token
	if err = windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return uiShutdownProcess{}, err
	}
	user, err := token.GetTokenUser()
	token.Close()
	if err != nil {
		return uiShutdownProcess{}, err
	}
	if !user.User.Sid.Equals(owner) {
		return uiShutdownProcess{}, nil
	}
	var path [32768]uint16
	n := uint32(len(path))
	if err = windows.QueryFullProcessImageName(h, 0, &path[0], &n); err != nil {
		return uiShutdownProcess{}, err
	}
	image := windows.UTF16ToString(path[:n])
	if err = verifyShutdownImage(base, image); err != nil {
		return uiShutdownProcess{}, err
	}
	keep = true
	return uiShutdownProcess{pid, h}, nil
}

func verifyShutdownImage(base, image string) error {
	for _, name := range []string{"current.json", "previous.json"} {
		r, err := loadNamed(base, name)
		if errors.Is(err, ErrMissing) {
			continue
		}
		if err != nil {
			return err
		}
		if !strings.EqualFold(image, r.Executable) {
			continue
		}
		f, hash, err := openExecutable(image)
		if err != nil {
			return err
		}
		closeErr := f.Close()
		if hash != r.SHA256 {
			return ErrInvalid
		}
		return closeErr
	}
	return ErrInvalid
}

func (s *UIShutdown) Close() error {
	var err error
	for _, p := range s.processes {
		err = errors.Join(err, windows.CloseHandle(p.handle))
	}
	s.processes = nil
	return err
}

func (s *UIShutdown) Stop(ctx context.Context) error {
	for _, p := range s.processes {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, err := windows.WaitForSingleObject(p.handle, 0)
		if err != nil {
			return err
		}
		if state == windows.WAIT_OBJECT_0 {
			continue
		}
		if err := postUIQuit(p.pid); err != nil {
			return err
		}
	}
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		done := true
		for _, p := range s.processes {
			state, err := windows.WaitForSingleObject(p.handle, 0)
			if err != nil {
				return err
			}
			if state != windows.WAIT_OBJECT_0 {
				done = false
			}
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// Wails g242+ already has this hidden main-loop window. WM_CLOSE only hides
// Krasis in the tray; WM_QUIT ends the application's existing message loop.
// Do not broadcast to windows, inject callbacks, or terminate any process.
func postUIQuit(pid uint32) error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	getPID := user32.NewProc("GetWindowThreadProcessId")
	getTitle := user32.NewProc("GetWindowTextW")
	post := user32.NewProc("PostThreadMessageW")
	var tid uint32
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var windowPID uint32
		thread, _, _ := getPID.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if windowPID == pid {
			var title [128]uint16
			getTitle.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
			if windows.UTF16ToString(title[:]) == "__wails_hidden_mainthread" {
				tid = uint32(thread)
				return 0
			}
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	if tid == 0 {
		return errors.New("Krasis main loop unavailable")
	}
	result, _, err := post.Call(uintptr(tid), 0x0012, 0, 0)
	if result == 0 {
		return err
	}
	return nil
}
