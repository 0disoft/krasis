//go:build windows

package inputworker

import (
	"context"
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
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, ErrAuthority
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
	limits.BasicLimitInformation.ActiveProcessLimit = 1
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)))
	if err != nil {
		_ = windows.CloseHandle(job)
		return 0, ErrAuthority
	}
	return job, nil
}

func createConsoleProcess(ctx context.Context, token windows.Token, path string, job windows.Handle, authorized func() bool, prepare func(windows.Handle, uint32) error) (windows.Handle, error) {
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
	command, err := windows.UTF16PtrFromString(`"` + path + `"` + workerArguments)
	if err != nil {
		return 0, ErrAuthority
	}
	directory, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return 0, ErrAuthority
	}
	desktop, _ := windows.UTF16PtrFromString(`winsta0\default`)
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
	defer windows.CloseHandle(child.Thread)
	accepted := false
	defer func() {
		if !accepted {
			_ = windows.TerminateProcess(child.Process, 1)
			_, _ = windows.WaitForSingleObject(child.Process, 5000)
			_ = windows.CloseHandle(child.Process)
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
	_ = identity.close()
	if !valid || ctx.Err() != nil || !authorized() {
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
