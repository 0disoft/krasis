package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"testing"
)

func TestLifecycleStopReasonsAreDistinctAndDoNotResumeOwner(t *testing.T) {
	for _, event := range []agentlifecycle.Event{agentlifecycle.EventSuspend, agentlifecycle.EventResume, agentlifecycle.EventSessionLock, agentlifecycle.EventSessionDisconnect, agentlifecycle.EventDisplayChange, agentlifecycle.EventDesktopSwitch, agentlifecycle.EventLogoff, agentlifecycle.EventShutdown} {
		t.Run(string(event), func(t *testing.T) {
			monitor := newRunMonitor()
			owner, err := agentlifecycle.Start(context.Background(), func() (agentlifecycle.Monitor, error) { return monitor, nil })
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close()
			var code string
			observed := observeLifecycleStop(owner.Context(), func(value string) { code = value })
			monitor.events <- event
			<-observed
			if code != "lifecycle_"+string(event) {
				t.Fatalf("wrong lifecycle reason: %s", code)
			}
			// A later resume cannot revive this owner or its old connection/lease.
			monitor.events <- agentlifecycle.EventResume
			if owner.Context().Err() == nil || owner.Event() != event {
				t.Fatal("old lifecycle owner revived")
			}
		})
	}
}

func TestOrdinaryStopDoesNotInventAnOSLifecycleEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var code string
	done := observeLifecycleStop(ctx, func(value string) { code = value })
	cancel()
	<-done
	if code != "" {
		t.Fatal(code)
	}
}
