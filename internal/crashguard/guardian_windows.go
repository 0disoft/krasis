//go:build windows

package crashguard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/winmessage"
	"golang.org/x/sys/windows"
)

// StartPortableGuardian is staged until the private agent mode and full pair
// coordinator are ready. Only the owned input Process supplies the identity;
// no PID lookup, peer-provided handle, token or service proof is accepted.
func StartPortableGuardian(executable string, input *Process) (*Process, error) {
	if !filepath.IsAbs(executable) {
		return nil, errors.New("absolute guardian executable required")
	}
	command := exec.Command(executable, "product-portable-guardian-internal")
	command.Dir = filepath.Dir(executable)
	return startPortableGuardian(command, input)
}

func startPortableGuardian(command *exec.Cmd, input *Process) (*Process, error) {
	if command == nil || input == nil || !input.portable || input.process == nil || input.closing.Load() || input.failed.Load() {
		return nil, errors.New("live owned portable input process required")
	}
	var parent, child windows.Handle
	access := uint32(windows.SYNCHRONIZE | windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.CurrentProcess(), windows.CurrentProcess(), &parent, access, true, 0); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(parent)
	var duplicateErr error
	err := input.process.WithHandle(func(handle uintptr) {
		duplicateErr = windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(handle), windows.CurrentProcess(), &child, access|windows.PROCESS_TERMINATE, true, 0)
	})
	if err = errors.Join(err, duplicateErr); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(child)
	// os/exec closes the inherited-handle allowlist. Neither child inherits
	// the other's parent-side stdio writers, so parent death also delivers EOF.
	command.SysProcAttr = &syscall.SysProcAttr{AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(parent), syscall.Handle(child)}}
	command.Args = append(command.Args, strconv.FormatUint(uint64(parent), 10), strconv.FormatUint(uint64(child), 10))
	return startCommand(command)
}

// ServePortableGuardian consumes the two handles from the closed local launch
// bootstrap. They are ordinary-user process references, never authorization.
// It owns both handles and joins the parent monitor before closing them.
func ServePortableGuardian(parent, input uintptr, reader io.ReadCloser, writer io.Writer, release application.ReleasePort) (Result, error) {
	if parent == 0 || input == 0 || parent == input || reader == nil || writer == nil || release == nil {
		return Result{}, errors.New("invalid guardian bootstrap")
	}
	parentHandle, inputHandle := windows.Handle(parent), windows.Handle(input)
	defer windows.CloseHandle(parentHandle)
	defer windows.CloseHandle(inputHandle)
	parentID, parentErr := windows.GetProcessId(parentHandle)
	inputID, inputErr := windows.GetProcessId(inputHandle)
	if err := errors.Join(parentErr, inputErr); err != nil {
		return Result{}, err
	}
	if parentID == inputID || parentID == windows.GetCurrentProcessId() || inputID == windows.GetCurrentProcessId() {
		return Result{}, errors.New("guardian process lifetimes must be distinct")
	}
	// No descendant may inherit these pinned references from the guardian.
	if err := windows.SetHandleInformation(parentHandle, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return Result{}, err
	}
	if err := windows.SetHandleInformation(inputHandle, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return Result{}, err
	}
	retirement := &guardianRetirement{input: inputHandle}
	ctx, cancel := context.WithCancel(context.Background())
	monitorDone := make(chan struct{})
	var monitorErr error
	go func() {
		defer close(monitorDone)
		err := waitPinnedGuardianProcess(ctx, parentHandle, windows.WaitForSingleObject)
		if errors.Is(err, context.Canceled) {
			return
		}
		// A failed parent proof also fences the input owner. Only the exact
		// input exit, never EOF or successful TerminateProcess, grants release.
		monitorErr = errors.Join(err, retirement.retire())
		monitorErr = errors.Join(monitorErr, reader.Close())
	}()
	result, err := ServeWithRetirement(reader, writer, release, retirement.retire)
	cancel()
	<-monitorDone
	return result, errors.Join(err, monitorErr)
}

type guardianRetirement struct {
	input windows.Handle
	once  sync.Once
	err   error
}

func (retirement *guardianRetirement) retire() error {
	retirement.once.Do(func() {
		killErr := windows.TerminateProcess(retirement.input, 1)
		waitErr := waitPinnedGuardianProcess(context.Background(), retirement.input, windows.WaitForSingleObject)
		if waitErr != nil {
			retirement.err = errors.Join(killErr, waitErr)
		}
		// A denied termination is harmless only after exact exit is proven.
	})
	return retirement.err
}

func waitPinnedGuardianProcess(ctx context.Context, handle windows.Handle, wait func(windows.Handle, uint32) (uint32, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, err := wait(handle, winmessage.MaximumWaitMillis)
		if err != nil {
			return fmt.Errorf("guardian process wait: %w", err)
		}
		switch state {
		case windows.WAIT_OBJECT_0:
			return nil
		case uint32(windows.WAIT_TIMEOUT):
			continue
		default:
			return fmt.Errorf("unexpected guardian process wait result %d", state)
		}
	}
}
