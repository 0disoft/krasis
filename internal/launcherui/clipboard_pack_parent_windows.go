//go:build windows

package launcherui

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

const clipboardPackParentEnvironment = "KRASIS_CLIPBOARD_PACK_PARENT"

// Transfer an actual, wait-only process reference through os/exec's explicit
// inherited-handle allowlist. A PID would permit reuse after the owner exits.
func inheritClipboardPackParent(command *exec.Cmd) (func(), error) {
	var parent windows.Handle
	err := windows.DuplicateHandle(windows.CurrentProcess(), windows.CurrentProcess(), windows.CurrentProcess(), &parent, windows.SYNCHRONIZE, true, 0)
	if err != nil {
		return nil, err
	}
	command.SysProcAttr.AdditionalInheritedHandles = append(command.SysProcAttr.AdditionalInheritedHandles, syscall.Handle(parent))
	command.Env = append(command.Environ(), clipboardPackParentEnvironment+"="+strconv.FormatUint(uint64(parent), 10))
	return func() { _ = windows.CloseHandle(parent) }, nil
}

// This disposable helper owns no input or product state. Owner death terminates
// the entire helper, including a thread blocked in native filesystem I/O. This
// reference grants no service, peer or desktop authority. Adopt it before any
// path parsing or filesystem work; never reopen the parent by PID.
func watchClipboardPackParent(value string) (func(), error) {
	number, err := strconv.ParseUint(value, 10, strconv.IntSize)
	if err != nil || number == 0 {
		return nil, errClipboardUnavailable
	}
	parent := windows.Handle(number)
	if err := windows.SetHandleInformation(parent, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, errClipboardUnavailable
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = windows.CloseHandle(parent)
		}
	}()
	if state, err := windows.WaitForSingleObject(parent, 0); err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return nil, errClipboardUnavailable
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, errClipboardUnavailable
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		state, err := windows.WaitForMultipleObjects([]windows.Handle{parent, stop}, false, windows.INFINITE)
		if err != nil || state != windows.WAIT_OBJECT_0+1 {
			os.Exit(2)
		}
	}()
	accepted = true
	return func() {
		if windows.SetEvent(stop) != nil {
			os.Exit(2)
		}
		<-done // No handle may close while the monitor is still waiting on it.
		_ = windows.CloseHandle(stop)
		_ = windows.CloseHandle(parent)
	}, nil
}
