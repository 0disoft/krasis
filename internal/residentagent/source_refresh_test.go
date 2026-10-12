package residentagent

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestProtectedSourceRefreshKeepsConnectionRunnerAlive(t *testing.T) {
	started := make(chan struct{})
	var refreshes, calls atomic.Int32
	o := testOperations(t, func(ctx context.Context, _ string, observer agent.SupervisorObserver) error {
		calls.Add(1)
		unregister := registerSourceRefresh(ctx, func(agentlifecycle.Event) bool {
			refreshes.Add(1)
			return true
		})
		defer unregister()
		observer.(interface{ ObserveInputBackend(bool) }).ObserveInputBackend(true)
		observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorActive})
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	if _, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); err != nil {
		t.Fatal(err)
	}
	awaitTransitionSignal(t, started)
	for _, event := range []agentlifecycle.Event{agentlifecycle.EventSessionLock, agentlifecycle.EventDesktopSwitch} {
		if !o.handleProtectedTransition(event) {
			t.Fatal("source refresh rejected", event)
		}
	}
	status, _ := o.Status(context.Background())
	if calls.Load() != 1 || refreshes.Load() != 2 || status.State != "CONNECTED" {
		t.Fatal("source refresh restarted connection", calls.Load(), refreshes.Load(), status)
	}
	if _, err := o.Disconnect(context.Background(), localapi.OperationRequest{}); err != nil {
		t.Fatal(err)
	}
	if o.handleProtectedTransition(agentlifecycle.EventDesktopSwitch) {
		t.Fatal("manual stop refreshed source")
	}
}

func TestSourceRefreshLateUnregisterCannotRemoveNewOwner(t *testing.T) {
	slot := &sourceRefreshSlot{}
	ctx := context.WithValue(context.Background(), sourceRefreshKey{}, slot)
	old := registerSourceRefresh(ctx, func(agentlifecycle.Event) bool { return false })
	current := registerSourceRefresh(ctx, func(agentlifecycle.Event) bool { return true })
	old()
	if !slot.handle(agentlifecycle.EventDesktopSwitch) {
		t.Fatal("late cleanup removed successor")
	}
	current()
	if slot.handle(agentlifecycle.EventDesktopSwitch) {
		t.Fatal("retired native owner remained reachable")
	}
}
