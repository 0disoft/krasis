package application

import (
	"context"
	"errors"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

var ErrInputLivenessExpired = errors.New("input arrived after peer liveness deadline")

// HandleInputActionWithLiveness records successful, exact-sequence inbound input
// as peer progress. It never renews a lease from an unvalidated or failed input.
func (runtime *SessionRuntime) HandleInputActionWithLiveness(ctx context.Context, envelope control.InputEnvelope, action control.InputAction, now func() time.Time, timeout time.Duration) error {
	if now == nil || timeout <= 0 {
		return errors.New("input observation clock and positive peer timeout are required")
	}
	return runtime.handleInputAction(ctx, envelope, action, now, timeout)
}

// Called under the runtime owner after action/session/epoch/sequence validation,
// before injection. Use admission time, not completion time, so a slow adapter
// cannot grant a fresh full lease after spending its injection budget.
func (runtime *SessionRuntime) inputLivenessLocked(ctx context.Context, now func() time.Time, timeout time.Duration) (control.PeerLiveness, time.Time, error) {
	if now == nil {
		return control.PeerLiveness{}, time.Time{}, nil
	}
	admittedAt := time.Now()
	observedAt := now()
	_, expired, err := runtime.liveness.FaultEventIfExpired(observedAt, timeout)
	if expired {
		err = ErrInputLivenessExpired
	}
	if err == nil {
		var next control.PeerLiveness
		next, err = runtime.liveness.Observe(runtime.control.SessionID, observedAt)
		if err == nil {
			return next, admittedAt, nil
		}
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), InputFailureCleanupTimeout)
	defer cancel()
	_, faultErr := runtime.faultLocked(cleanupCtx, err)
	return control.PeerLiveness{}, time.Time{}, faultErr
}
