package agent

import (
	"errors"
	"time"
)

// A logical LOCAL state alone does not prove native input was released.
var ErrRecoveryUnverified = errors.New("agent input recovery is unverified")

type RecoveryEvidence uint8

const (
	RecoveryUnknown RecoveryEvidence = iota
	RecoveryConfirmed
)

type RecoveryReport struct {
	ForwardingStopped    RecoveryEvidence
	WorkersJoined        RecoveryEvidence
	NativeOwnersReleased RecoveryEvidence
	PressedLedgerEmpty   RecoveryEvidence
	DeadlineExceeded     bool
}

// Bound the reporting deadline, not the lifetime of unfinished input owners.
// Returning while an owner can still inject would let outer guard cleanup race
// a late key-down. Keep that owner and guard alive, but publish FAULTED promptly.
func awaitRecovery(joined <-chan struct{}, budget time.Duration, observe func(RecoveryReport)) bool {
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case <-joined:
		return false
	case <-timer.C:
		if observe != nil {
			observe(RecoveryReport{ForwardingStopped: RecoveryConfirmed, DeadlineExceeded: true})
		}
		<-joined
		return true
	}
}
