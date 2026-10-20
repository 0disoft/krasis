//go:build windows

package inputworker

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	workerArguments = " --input-worker"
	// Win32 PROC_THREAD_ATTRIBUTE_JOB_LIST; not exported by x/sys v0.46.
	procThreadAttributeJobList = 0x0002000d
)

func makeWorkerJob() (windows.Handle, error) {
	return makeWorkerJobLimit(1)
}

func makeConsoleWorkerJob(mode consoleMode) (windows.Handle, error) {
	if mode == ordinaryConsole || mode == loginConsole {
		return makeWorkerJobLimit(2)
	}
	return 0, ErrAuthority
}

func makeWorkerJobLimit(limit uint32) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, ErrAuthority
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	limits.BasicLimitInformation.ActiveProcessLimit = limit
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, ErrAuthority
	}
	return job, nil
}

func createConsoleProcess(ctx context.Context, token windows.Token, path string, job windows.Handle, authorized func() bool, prepare func(windows.Handle, uint32) error) (windows.Handle, error) {
	return createManagedConsoleProcess(ctx, token, path, job, ordinaryConsole, authorized, prepare)
}

// Closed choices owned by the service, never desktop/command strings from IPC.
type consoleMode uint8

const (
	ordinaryConsole consoleMode = iota
	loginConsole
)

func consoleLaunchMode(mode consoleMode) (arguments, desktop string, err error) {
	switch mode {
	case ordinaryConsole:
		return workerArguments, `winsta0\default`, nil
	case loginConsole:
		return " --login-worker", `winsta0\Winlogon`, nil
	default:
		return "", "", ErrAuthority
	}
}

func createManagedConsoleProcess(ctx context.Context, token windows.Token, path string, job windows.Handle, mode consoleMode, authorized func() bool, prepare func(windows.Handle, uint32) error) (result windows.Handle, resultErr error) {
	arguments, desktopName, err := consoleLaunchMode(mode)
	if err != nil || ctx == nil || ctx.Err() != nil || job == 0 || authorized == nil || !authorized() {
		return 0, ErrAuthority
	}
	attributes, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return 0, ErrAuthority
	}
	var pinned runtime.Pinner
	pinned.Pin(&job)
	defer func() { attributes.Delete(); pinned.Unpin() }()
	// Windows assigns the job during creation, removing the crash interval
	// between process creation and a later AssignProcessToJobObject call.
	if attributes.Update(procThreadAttributeJobList, unsafe.Pointer(&job), unsafe.Sizeof(job)) != nil {
		return 0, ErrAuthority
	}
	app, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, ErrAuthority
	}
	command, err := windows.UTF16PtrFromString(`"` + path + `"` + arguments)
	if err != nil {
		return 0, ErrAuthority
	}
	directory, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return 0, ErrAuthority
	}
	desktop, _ := windows.UTF16PtrFromString(desktopName)
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Desktop: desktop}, ProcThreadAttributeList: attributes.List()}
	var environment [2]uint16 // Double-NUL: no inherited PATH, secrets or profile.
	var child windows.ProcessInformation
	if ctx.Err() != nil || !authorized() {
		return 0, ErrAuthority
	}
	err = windows.CreateProcessAsUser(token, app, command, nil, nil, false,
		windows.CREATE_SUSPENDED|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW|windows.EXTENDED_STARTUPINFO_PRESENT,
		&environment[0], directory, &startup.StartupInfo, &child)
	runtime.KeepAlive(job)
	if err != nil {
		return 0, ErrAuthority
	}
	accepted := false
	defer func() {
		if windows.CloseHandle(child.Thread) != nil {
			accepted = false
			result, resultErr = 0, errors.Join(resultErr, ErrRecovery)
		}
		if !accepted {
			// Return ownership even on rejection. The caller's ConsoleWorker
			// owns the job and image pin and performs bounded retirement. Never
			// discard the exact process handle after an unconfirmed local wait.
			result, resultErr = child.Process, errors.Join(resultErr, ErrAuthority)
		}
	}()
	// Verify the actual child's token while it is suspended. Arguments and a
	// successful CreateProcessAsUser call are not proof of the resulting token.
	identity, err := inspectProcess(child.ProcessId)
	if err != nil {
		return 0, ErrAuthority
	}
	session := windows.WTSGetActiveConsoleSessionId()
	valid := session != 0 && session != 0xffffffff && identity.system && identity.uiAccess && identity.integrity == 0x4000 &&
		identity.session == session && !identity.restricted && !identity.appContainer && strings.EqualFold(identity.image, path)
	identityCloseErr := identity.close()
	if !valid || identityCloseErr != nil || ctx.Err() != nil || !authorized() {
		return 0, ErrAuthority
	}
	if prepare != nil && prepare(child.Process, child.ProcessId) != nil {
		return 0, ErrAuthority
	}
	if ctx.Err() != nil || !authorized() {
		return 0, ErrAuthority
	}
	previous, err := windows.ResumeThread(child.Thread)
	if err != nil || previous != 1 {
		return 0, ErrAuthority
	}
	accepted = true
	return child.Process, nil
}
