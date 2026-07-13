package winlifecycle

import (
	"testing"

	"github.com/0disoft/krasis/internal/agentlifecycle"
)

func TestEventFromMessageMapsSafetyRelevantTransitions(t *testing.T) {
	tests := []struct {
		name    string
		message uint32
		wParam  uintptr
		want    agentlifecycle.Event
	}{
		{name: "suspend", message: wmPowerBroadcast, wParam: pbtAPMSuspend, want: agentlifecycle.EventSuspend},
		{name: "automatic resume", message: wmPowerBroadcast, wParam: pbtAPMResumeAutomatic, want: agentlifecycle.EventResume},
		{name: "session lock", message: wmWTSSessionChange, wParam: wtsSessionLock, want: agentlifecycle.EventSessionLock},
		{name: "remote disconnect", message: wmWTSSessionChange, wParam: wtsRemoteDisconnect, want: agentlifecycle.EventSessionDisconnect},
		{name: "logoff", message: wmWTSSessionChange, wParam: wtsSessionLogoff, want: agentlifecycle.EventLogoff},
		{name: "query shutdown", message: wmQueryEndSession, want: agentlifecycle.EventShutdown},
		{name: "confirmed shutdown", message: wmEndSession, wParam: 1, want: agentlifecycle.EventShutdown},
		{name: "display change", message: wmDisplayChange, want: agentlifecycle.EventDisplayChange},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := eventFromMessage(test.message, test.wParam)
			if !ok || got != test.want {
				t.Fatalf("eventFromMessage() = %q, %t; want %q", got, ok, test.want)
			}
		})
	}
}

func TestEventFromMessageIgnoresNonTerminalMessages(t *testing.T) {
	for _, test := range []struct {
		message uint32
		wParam  uintptr
	}{
		{message: wmPowerBroadcast, wParam: 0},
		{message: wmWTSSessionChange, wParam: 5},
		{message: wmEndSession, wParam: 0},
		{message: 0x0400, wParam: 1},
	} {
		if event, ok := eventFromMessage(test.message, test.wParam); ok || event != "" {
			t.Fatalf("unexpected event %q for message %#x/%#x", event, test.message, test.wParam)
		}
	}
}
