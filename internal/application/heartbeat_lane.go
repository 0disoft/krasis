package application

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

// ObserveHeartbeatAhead is for the sole authenticated wire reader. Only an
// already active, exact, unexpired lease may bypass the input owner. Pending,
// retired and different-session frames must retain normal ordered dispatch.
// CAS against the entire immutable watch fences concurrent revocation/regrant.
// It never admits input, grants a lease, performs I/O, or executes recovery.
func (runtime *SessionRuntime) ObserveHeartbeatAhead(ctx context.Context, sessionID string, observedAt time.Time, timeout time.Duration) (bool, error) {
	if ctx == nil || timeout <= 0 {
		return false, control.ErrInvalidLiveness
	}
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		previous := runtime.watch.Load()
		if previous == nil || previous.Sealed || previous.State != control.StateInboundActive && previous.State != control.StateOutboundActive || previous.SessionID != sessionID || previous.Liveness.SessionID == "" {
			return false, nil
		}
		_, expired, err := previous.Liveness.FaultEventIfExpired(observedAt, timeout)
		if err != nil {
			return false, err
		}
		if expired || time.Since(previous.Progress) >= timeout {
			return false, ErrInputLivenessExpired
		}
		liveness, err := previous.Liveness.Observe(sessionID, observedAt)
		if err != nil {
			return false, err
		}
		next := *previous
		next.Liveness, next.Progress = liveness, time.Now()
		if ctx.Err() != nil || next.Progress.Sub(previous.Progress) >= timeout {
			return false, ErrInputLivenessExpired
		}
		if runtime.watch.CompareAndSwap(previous, &next) {
			return true, nil
		}
	}
}

// Only the serialized runtime owner calls these helpers. The atomic copy can
// advance while native input is joined; an old input receipt cannot overwrite
// a newer heartbeat or publish a different lease's progress.
func (runtime *SessionRuntime) syncLivenessLocked() {
	watch := runtime.watch.Load()
	if watch != nil && watch.State == runtime.control.State && watch.SessionID == runtime.control.SessionID && watch.Liveness.SessionID == runtime.liveness.SessionID && watch.Liveness.LastObserved.After(runtime.liveness.LastObserved) {
		runtime.liveness = watch.Liveness
	}
}

func (runtime *SessionRuntime) recordLivenessLocked(liveness control.PeerLiveness, progress time.Time) {
	for {
		previous := runtime.watch.Load()
		if previous == nil || previous.Sealed || previous.State != runtime.control.State || previous.SessionID != liveness.SessionID {
			return
		}
		next := *previous
		if next.Liveness.LastObserved.Before(liveness.LastObserved) {
			next.Liveness = liveness
		}
		if next.Progress.Before(progress) {
			next.Progress = progress
		}
		if runtime.watch.CompareAndSwap(previous, &next) {
			runtime.liveness = next.Liveness
			return
		}
	}
}

// Healthy checks require no input mutex. The caller still owns semantic write
// ordering; expired checks/recovery continue through CheckLiveness's owner.
func (runtime *SessionRuntime) HeartbeatLease(clock func() time.Time, timeout time.Duration) (LeaseWatch, bool, time.Time, error) {
	watch := runtime.LeaseWatch()
	if watch.State != control.StateInboundActive && watch.State != control.StateOutboundActive {
		return watch, false, time.Time{}, nil
	}
	now := clock() // Read after the proof; concurrent progress cannot make this clock stale.
	if watch.SessionID != watch.Liveness.SessionID {
		return watch, true, now, control.ErrInvalidLiveness
	}
	_, expired, err := watch.Liveness.FaultEventIfExpired(now, timeout)
	if watch.Sealed || expired || timeout > 0 && time.Since(watch.Progress) >= timeout {
		err = ErrInputLivenessExpired
	}
	return watch, true, now, err
}

// The watchdog must claim the exact expired proof, not stop a lease which a
// concurrent valid heartbeat has already renewed. Sealing precedes native
// cleanup, so a delayed CAS can never revive an expiry already acted upon.
func (runtime *SessionRuntime) ClaimLeaseExpiry(watch LeaseWatch, budget time.Duration) bool {
	previous := runtime.watch.Load()
	if previous == nil || *previous != watch || budget <= 0 || time.Since(watch.Progress) < budget {
		return false
	}
	return previous.Sealed || runtime.sealLeaseWatch(previous)
}

func (runtime *SessionRuntime) sealLeaseWatch(previous *LeaseWatch) bool {
	next := *previous
	next.Sealed = true
	return runtime.watch.CompareAndSwap(previous, &next)
}
