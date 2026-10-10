package loginagent

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/wininput"
)

func runLoginSession(ctx context.Context, config agent.Config, deps agent.Dependencies) (agent.Result, error) {
	result, err := agent.RunSession(ctx, config, deps)
	return result, loginSessionRetirement(ctx, result, err)
}

// Successful retirement is about owned work, not the last connection error.
// Only the fully confirmed session report may turn local cancellation into a
// clean stop. Outer connection/source-close and Winlogon journal errors remain.
func loginSessionRetirement(ctx context.Context, result agent.Result, err error) error {
	r := result.Recovery
	if ctx.Err() != nil && result.FinalState == control.StateLocal && !errors.Is(err, agent.ErrRecoveryUnverified) &&
		r.ForwardingStopped == agent.RecoveryConfirmed && r.WorkersJoined == agent.RecoveryConfirmed &&
		r.NativeOwnersReleased == agent.RecoveryConfirmed && r.PressedLedgerEmpty == agent.RecoveryConfirmed && !r.DeadlineExceeded {
		return ctx.Err()
	}
	return err
}

// Recursive inspection is deliberate: errors.Is(Canceled) alone would hide a
// joined cleanup failure. Policy/geometry revocation is a clean reason to stop,
// but unknown I/O, token, native release and process-reaping errors are not.
func loginStopOnly(err error) bool {
	if err == nil {
		return true
	}
	if agent.SupervisorExitPhase(err) != "" {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if !loginStopOnly(child) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		if child := wrapped.Unwrap(); child != nil {
			return loginStopOnly(child)
		}
	}
	return err == context.Canceled || err == context.DeadlineExceeded || err == ErrGrant || err == ErrObservation || err == ErrSuspended || err == wininput.ErrDisplayGeometryChanged
}
