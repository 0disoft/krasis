//go:build windows

package inputworker

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Both values come from Windows, never a peer frame. The service handle refers
// to the protected configuration validated by AuthenticateService and stays
// owned by that channel's Authority.
func readServicePipeProcess(pipe, service windows.Handle) (uint32, windows.SERVICE_STATUS_PROCESS, error) {
	var pid uint32
	var status windows.SERVICE_STATUS_PROCESS
	if err := windows.GetNamedPipeServerProcessId(pipe, &pid); err != nil {
		return 0, status, connectionFailed("service_pid", err)
	}
	var needed uint32
	if err := windows.QueryServiceStatusEx(service, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&status)), uint32(unsafe.Sizeof(status)), &needed); err != nil {
		return 0, status, connectionFailed("service_status", err)
	}
	return pid, status, nil
}

// Bind one already-verified service to its actual kernel pipe owner. A change
// of PID or a pending/stopped service invalidates the proof. Broken-pipe I/O
// remains fatal even if SCM has not yet observed an exit or a PID is reused.
// No OpenProcess, token query, privilege enablement or ACL write is required.
func serviceProcessBinding(expected uint32, read func() (uint32, windows.SERVICE_STATUS_PROCESS, error)) func() error {
	return func() error {
		if expected == 0 || read == nil {
			return connectionFailed("service_status", ErrAuthority)
		}
		pid, status, err := read()
		if err != nil {
			return err
		}
		if pid != expected || status.ProcessId != expected || status.CurrentState != windows.SERVICE_RUNNING || status.ServiceType != windows.SERVICE_WIN32_OWN_PROCESS {
			return connectionFailed("service_status", ErrAuthority)
		}
		return nil
	}
}
