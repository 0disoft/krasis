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
	return prepareUIShutdown(base, 0)
}

// A verified replacement may close only older management windows. The current
// process, same-generation windows and newer windows remain untouched.
func PrepareOlderUIShutdown(base string, generation int) (*UIShutdown, error) {
	if generation <= 0 {
		return nil, ErrInvalid
	}
	return prepareUIShutdown(base, generation)
}

func (s *UIShutdown) HasProcesses() bool { return len(s.processes) != 0 }

func prepareUIShutdown(base string, beforeGeneration int) (*UIShutdown, error) {
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
		if entry.ProcessID != windows.GetCurrentProcessId() && strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "krasis.exe") {
			var targetSession uint32
			if e := windows.ProcessIdToSessionId(entry.ProcessID, &targetSession); e != nil {
				plan.Close()
				return nil, e
			}
			if targetSession == session {
				p, e := prepareShutdownProcess(base, entry.ProcessID, me.User.Sid, beforeGeneration)
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

func prepareShutdownProcess(base string, pid uint32, owner *windows.SID, beforeGeneration int) (uiShutdownProcess, error) {
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
	record, err := verifiedShutdownImage(base, image)
	if err != nil {
		return uiShutdownProcess{}, err
	}
	if beforeGeneration != 0 && record.Generation >= beforeGeneration {
		return uiShutdownProcess{}, nil
	}
	keep = true
	return uiShutdownProcess{pid, h}, nil
}

func verifyShutdownImage(base, image string) error {
	_, err := verifiedShutdownImage(base, image)
	return err
}

func verifiedShutdownImage(base, image string) (Record, error) {
	for _, name := range []string{"current.json", "previous.json"} {
		r, err := loadNamed(base, name)
		if errors.Is(err, ErrMissing) {
			continue
		}
		if err != nil {
			return Record{}, err
		}
		if !strings.EqualFold(image, r.Executable) {
			continue
		}
		f, hash, err := openExecutable(image)
		if err != nil {
			return Record{}, err
		}
		closeErr := f.Close()
		if hash != r.SHA256 {
			return Record{}, ErrInvalid
		}
		return r, closeErr
	}
	return Record{}, ErrInvalid
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

// Wails g242+ already has this hidden main-loop window. WM_CLOSE on this
// window invokes App.Quit and its shutdown callbacks. On the visible window it
// merely hides Krasis; WM_QUIT skips callbacks and can strand service shutdown.
// Do not broadcast to windows, inject callbacks, or terminate any process.
func postUIQuit(pid uint32) error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	getPID := user32.NewProc("GetWindowThreadProcessId")
	getTitle := user32.NewProc("GetWindowTextW")
	post := user32.NewProc("PostMessageW")
	var target uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var windowPID uint32
		getPID.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if windowPID == pid {
			var title [128]uint16
			getTitle.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
			if windows.UTF16ToString(title[:]) == "__wails_hidden_mainthread" {
				target = hwnd
				return 0
			}
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	if target == 0 {
		return errors.New("Krasis main loop unavailable")
	}
	result, _, err := post.Call(target, 0x0010, 0, 0)
	if result == 0 {
		return err
	}
	return nil
}
