package application

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

const LeaseHandshakeTimeout = 5 * time.Second

var ErrPendingLeaseExpired = errors.New("pending lease preparation expired")

// ValidateOutboundPreparation authorizes capture preparation without starting
// active liveness. Call again after native preparation: cancellation or expiry
// must never turn a late native success into a new active lease.
func (runtime *SessionRuntime) ValidateOutboundPreparation(ctx context.Context, sessionID, leaseID, epoch string, observedAt time.Time) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.control.State != control.StatePending || runtime.outbound.SessionID != sessionID || runtime.outbound.Epoch != epoch || sessionID == "" || leaseID == "" {
		return errors.New("outbound preparation does not match a pending grant")
	}
	if _, err := control.NewPeerLiveness(sessionID, observedAt); err != nil {
		return err
	}
	watch := runtime.watch.Load()
	if watch == nil || watch.Progress.IsZero() || time.Since(watch.Progress) >= LeaseHandshakeTimeout {
		return ErrPendingLeaseExpired
	}
	return nil
}
