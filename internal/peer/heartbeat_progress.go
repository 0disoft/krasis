package peer

import "time"

// Progress is diagnostic only. Send: coordinator queue/acquire, TLS write
// start, terminal. Receive: complete frame read, lease admission (atomic for
// an active lease; coordinated otherwise), terminal. Complete certifies a
// successful local write or exact-lease heartbeat admission, never input apply.
type HeartbeatStage uint8

const (
	HeartbeatStart HeartbeatStage = iota
	HeartbeatGate
	HeartbeatWork
	HeartbeatComplete
	HeartbeatSkipped
	HeartbeatFailed
)

type HeartbeatSendObserver interface {
	// Callbacks must not wait, allocate, perform I/O or affect lease authority.
	ObserveHeartbeatSend(HeartbeatStage, time.Time)
}

type HeartbeatReceiveObserver interface {
	ObserveHeartbeatReceive(HeartbeatStage, time.Time)
}
