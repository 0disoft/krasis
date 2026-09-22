//go:build windows

package winlifecycle

import (
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"sync"
	"syscall"
)

const eventSystemDesktopSwitch = 0x0020

var desktopSwitchOwners sync.Map
var setDesktopEventHook = user32.NewProc("SetWinEventHook")
var unhookDesktopEvent = user32.NewProc("UnhookWinEvent")

// OUTOFCONTEXT delivers on the installing message-loop thread. No DLL is injected
// and no secure desktop content, desktop name or target window is read.
var desktopSwitchCallback = syscall.NewCallback(dispatchDesktopSwitch)

func dispatchDesktopSwitch(hook, event, window, object, child, thread, timestamp uintptr) uintptr {
	if event == eventSystemDesktopSwitch {
		if owner, ok := desktopSwitchOwners.Load(hook); ok {
			owner.(*Monitor).publish(agentlifecycle.EventDesktopSwitch)
		}
	}
	return 0
}

func (monitor *Monitor) watchDesktopSwitch() (func(), error) {
	hook, _, err := setDesktopEventHook.Call(eventSystemDesktopSwitch, eventSystemDesktopSwitch, 0, desktopSwitchCallback, 0, 0, 0)
	if hook == 0 {
		return nil, win32Error("SetWinEventHook(desktop switch)", err)
	}
	desktopSwitchOwners.Store(hook, monitor)
	return func() {
		if ok, _, err := unhookDesktopEvent.Call(hook); ok == 0 {
			monitor.setRunError(win32Error("UnhookWinEvent(desktop switch)", err))
		}
		desktopSwitchOwners.CompareAndDelete(hook, monitor)
	}, nil
}
