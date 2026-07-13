package agentlifecycle

import "fmt"

type Event string

const (
	EventSuspend           Event = "suspend"
	EventResume            Event = "resume"
	EventSessionLock       Event = "session_lock"
	EventSessionDisconnect Event = "session_disconnect"
	EventLogoff            Event = "logoff"
	EventShutdown          Event = "shutdown"
	EventDisplayChange     Event = "display_change"
)

func (event Event) Validate() error {
	switch event {
	case EventSuspend, EventResume, EventSessionLock, EventSessionDisconnect,
		EventLogoff, EventShutdown, EventDisplayChange:
		return nil
	default:
		return fmt.Errorf("unsupported agent lifecycle event %q", event)
	}
}

type TransitionError struct {
	Event Event
}

func (err *TransitionError) Error() string {
	return fmt.Sprintf("agent restart required after %s", err.Event)
}
