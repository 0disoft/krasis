package peer

import (
	"github.com/0disoft/krasis/internal/transport"
	"time"
)

// SessionProgressObserver records stages only. Implementations must not wait or
// perform I/O. Receiving a frame does not validate or refresh its lease.
type SessionProgressObserver interface {
	ObserveReadStarted(time.Time)
	ObserveReadCompleted(time.Time)
	ObserveDispatchStarted(transport.MessageKind, time.Time)
	ObserveDispatchCompleted(time.Time)
}

type TimerObserver interface {
	ObserveTimerLateness(time.Duration)
}
