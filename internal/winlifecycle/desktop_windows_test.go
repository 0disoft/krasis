//go:build windows

package winlifecycle

import (
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"testing"
)

func TestDesktopSwitchRoutesOnlyRegisteredOwnerAndIgnoresLateCallbacks(t *testing.T) {
	monitor := &Monitor{events: make(chan agentlifecycle.Event, 1)}
	hook := ^uintptr(0)
	desktopSwitchOwners.Store(hook, monitor)
	defer desktopSwitchOwners.Delete(hook)
	dispatchDesktopSwitch(hook, eventSystemDesktopSwitch+1, 0, 0, 0, 0, 0)
	if len(monitor.events) != 0 {
		t.Fatal("unrelated event treated as desktop switch")
	}
	dispatchDesktopSwitch(hook, eventSystemDesktopSwitch, 0, 0, 0, 0, 0)
	if got := <-monitor.events; got != agentlifecycle.EventDesktopSwitch {
		t.Fatal(got)
	}
	desktopSwitchOwners.Delete(hook)
	close(monitor.events)
	dispatchDesktopSwitch(hook, eventSystemDesktopSwitch, 0, 0, 0, 0, 0) // Must not send to retired channel.
}
