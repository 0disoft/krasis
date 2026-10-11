package residentagent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestProtectedDesktopTransitionJoinsCleanupBeforeRestart(t *testing.T) {
	started := make(chan int, 2)
	cleaning := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	o := testOperations(t, func(ctx context.Context, _ string, observer agent.SupervisorObserver) error {
		call := int(calls.Add(1))
		observer.(interface{ ObserveInputBackend(bool) }).ObserveInputBackend(true)
		observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorActive})
		started <- call
		<-ctx.Done()
		if call == 1 {
			observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorStopped})
			close(cleaning)
			<-release
		}
		return ctx.Err()
	})
	if result, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); err != nil || result.Status != "COMPLETED" {
		t.Fatal(result, err)
	}
	awaitTransitionSignal(t, started)
	if !o.handleProtectedTransition(agentlifecycle.EventSessionLock) {
		t.Fatal("verified protected lock retired the process")
	}
	awaitTransitionSignal(t, cleaning)
	if !o.handleProtectedTransition(agentlifecycle.EventDesktopSwitch) {
		t.Fatal("duplicate desktop change during cleanup retired the process")
	}
	if o.Context().Err() != nil || calls.Load() != 1 {
		t.Fatal("process cancelled or replacement started before cleanup")
	}
	close(release)
	if call := awaitTransitionSignal(t, started); call != 2 {
		t.Fatal("replacement missing", call)
	}
	if status, _ := o.Status(context.Background()); status.State != "CONNECTED" || status.InputBackend != "PROTECTED" {
		t.Fatal(status)
	}
	if result, err := o.Disconnect(context.Background(), localapi.OperationRequest{}); err != nil || result.Status != "COMPLETED" {
		t.Fatal(result, err)
	}
	if calls.Load() != 2 || o.handleProtectedTransition(agentlifecycle.EventSessionLock) {
		t.Fatal("manual stop permitted another automatic restart")
	}
}

func TestProtectedDesktopTransitionDoesNotHideCleanupFailure(t *testing.T) {
	started := make(chan struct{})
	cleanupFailure := errors.New("input release unconfirmed")
	var calls atomic.Int32
	o := testOperations(t, func(ctx context.Context, _ string, observer agent.SupervisorObserver) error {
		calls.Add(1)
		observer.(interface{ ObserveInputBackend(bool) }).ObserveInputBackend(true)
		close(started)
		<-ctx.Done()
		return errors.Join(ctx.Err(), cleanupFailure)
	})
	if result, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); err != nil || result.Status != "COMPLETED" {
		t.Fatal(result, err)
	}
	awaitTransitionSignal(t, started)
	o.mu.Lock()
	done := o.sessionDone
	o.mu.Unlock()
	if !o.handleProtectedTransition(agentlifecycle.EventDesktopSwitch) {
		t.Fatal("transition not handled")
	}
	awaitTransitionSignal(t, done)
	if status, _ := o.Status(context.Background()); status.State != "FAULTED" || calls.Load() != 1 {
		t.Fatal("cleanup failure retried or hidden", status, calls.Load())
	}
}

func TestDesktopTransitionRequiresVerifiedLiveProtectedSession(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		backend                 string
		manual, fault, updating bool
		event                   agentlifecycle.Event
	}{
		{name: "portable", backend: "PORTABLE", event: agentlifecycle.EventSessionLock},
		{name: "unverified", backend: "UNKNOWN", event: agentlifecycle.EventDesktopSwitch},
		{name: "manual stop", backend: "PROTECTED", manual: true, event: agentlifecycle.EventSessionLock},
		{name: "failed cleanup", backend: "PROTECTED", fault: true, event: agentlifecycle.EventSessionLock},
		{name: "update", backend: "PROTECTED", updating: true, event: agentlifecycle.EventSessionLock},
		{name: "logoff", backend: "PROTECTED", event: agentlifecycle.EventLogoff},
		{name: "suspend", backend: "PROTECTED", event: agentlifecycle.EventSuspend},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := testOperations(t, nil)
			o.inputBackend, o.manualStop, o.sessionFaulted, o.updateApplying = tc.backend, tc.manual, tc.fault, tc.updating
			o.sessionDone = make(chan struct{})
			o.sessionTransition = func(error) { t.Fatal("unapproved session cancelled") }
			if o.handleProtectedTransition(tc.event) {
				t.Fatal("unapproved continuation")
			}
		})
	}
}

func awaitTransitionSignal[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(2 * time.Second):
		t.Fatal("desktop transition did not complete")
		var zero T
		return zero
	}
}
