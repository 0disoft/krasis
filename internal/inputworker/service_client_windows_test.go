//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
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

func TestObservedServiceBindingUsesSameDenialsAndQueriesOnce(t *testing.T) {
	for _, state := range []uint32{windows.SERVICE_RUNNING, windows.SERVICE_START_PENDING, windows.SERVICE_STOP_PENDING, windows.SERVICE_STOPPED} {
		calls := 0
		check := serviceProcessBindingObserved(42, func(context.Context) (uint32, windows.SERVICE_STATUS_PROCESS, error) {
			calls++
			return 42, windows.SERVICE_STATUS_PROCESS{ProcessId: 42, CurrentState: state, ServiceType: windows.SERVICE_WIN32_OWN_PROCESS}, nil
		})
		if (check(context.Background()) == nil) != (state == windows.SERVICE_RUNNING) || calls != 1 {
			t.Fatal("observed binding skipped, repeated or relaxed checks")
		}
	}
	if serviceProcessBindingObserved(42, nil)(context.Background()) == nil {
		t.Fatal("missing observed query accepted")
	}
}

func TestObservedNativeQueryKeepsFailureAndPresence(t *testing.T) {
	var result handoff.Summary
	c := handoff.New(func(s handoff.Summary) { result = s })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "receiver", "session", "epoch", time.Now())
	handoff.Backend(p, "installed")
	x, complete := handoff.Input(ctx, "receiver")
	server, client := nativePair(t)
	defer server.Close()
	defer client.Close()
	_, _, err := readServicePipeProcessObserved(x, client.handle, 0)
	if err == nil || ConnectionFailureEvent(err) != "installed_input_stage_service_status" {
		t.Fatal("observed queries accepted pipe-only proof")
	}
	complete(err)
	handoff.Finish(p, "error")
	if !result.Valid() || result.First.AuthorityMeasured != 6 {
		t.Fatal("failed query timings absent or malformed", result)
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
