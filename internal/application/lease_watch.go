package application

import (
	"github.com/0disoft/krasis/internal/control"
	"time"
)

// LeaseWatch is immutable and readable without the input/runtime mutex. Its
// monotonic clock is independent of protocol timestamps and diagnostic clocks.
type LeaseWatch struct {
	State     control.State
	SessionID string
	Progress  time.Time
}

func (runtime *SessionRuntime) LeaseWatch() LeaseWatch {
	if value := runtime.watch.Load(); value != nil {
		return *value
	}
	return LeaseWatch{State: control.StateLocal}
}

func (runtime *SessionRuntime) publishLeaseWatchLocked(heartbeat bool) {
	previous := runtime.watch.Load()
	if !heartbeat && previous != nil && previous.State == runtime.control.State && previous.SessionID == runtime.control.SessionID {
		return
	}
	runtime.watch.Store(&LeaseWatch{State: runtime.control.State, SessionID: runtime.control.SessionID, Progress: time.Now()})
}
