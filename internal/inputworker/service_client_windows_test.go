//go:build windows

package inputworker

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestServicePipeProofRejectsRestartAndNonRunningStates(t *testing.T) {
	const pid uint32 = 42
	pipePID := pid
	status := windows.SERVICE_STATUS_PROCESS{ProcessId: pid, CurrentState: windows.SERVICE_RUNNING, ServiceType: windows.SERVICE_WIN32_OWN_PROCESS}
	check := serviceProcessBinding(pid, func() (uint32, windows.SERVICE_STATUS_PROCESS, error) { return pipePID, status, nil })
	if err := check(); err != nil {
		t.Fatal("matching running own-process service rejected")
	}
	for _, state := range []uint32{windows.SERVICE_START_PENDING, windows.SERVICE_STOP_PENDING, windows.SERVICE_STOPPED, windows.SERVICE_PAUSED} {
		status.CurrentState = state
		if err := check(); !errors.Is(err, ErrAuthority) {
			t.Fatal("non-running service retained authority")
		}
	}
	status.CurrentState = windows.SERVICE_RUNNING
	status.ProcessId++
	if err := check(); !errors.Is(err, ErrAuthority) {
		t.Fatal("restarted service retained old pipe authority")
	}
	status.ProcessId = pid
	pipePID++
	if err := check(); !errors.Is(err, ErrAuthority) {
		t.Fatal("foreign kernel pipe owner accepted")
	}
	pipePID = pid
	status.ServiceType = windows.SERVICE_WIN32_SHARE_PROCESS
	if err := check(); !errors.Is(err, ErrAuthority) {
		t.Fatal("shared service process accepted")
	}
}

func TestServicePipeProofRequiresKernelAndSCMQueries(t *testing.T) {
	if err := serviceProcessBinding(0, nil)(); !errors.Is(err, ErrAuthority) {
		t.Fatal("missing service proof accepted")
	}
	cause := connectionFailed("service_status", windows.ERROR_ACCESS_DENIED)
	check := serviceProcessBinding(42, func() (uint32, windows.SERVICE_STATUS_PROCESS, error) {
		return 42, windows.SERVICE_STATUS_PROCESS{}, cause
	})
	if err := check(); !errors.Is(err, cause) || ConnectionNativeFailureEvent(err) != "installed_input_native_5" {
		t.Fatal("failed SCM query lost its authority failure or diagnostic")
	}
	server, client := nativePair(t)
	defer server.Close()
	// A real native pipe and its kernel PID cannot stand in for SCM authority.
	if _, _, err := readServicePipeProcess(client.handle, 0); err == nil || ConnectionFailureEvent(err) != "installed_input_stage_service_status" {
		t.Fatal("pipe-only proof accepted without SCM authority")
	}
}
