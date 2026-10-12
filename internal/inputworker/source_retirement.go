package inputworker

import (
	"context"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

// A final authenticated retirement proves native owners joined. Cancellation
// and EOF may become ready together; select order must not change that proof.
func sourceRetirementResult(ctx context.Context, retired bool, cause error) (error, bool) {
	if !retired || cause != nil {
		return cause, false
	}
	if ctx.Err() != nil {
		return ctx.Err(), true
	}
	return wininput.ErrProtectedInput, true
}

// A known inactive desktop is normal only after all native owners have joined.
// Joined cleanup/close failures must not inherit its normal classification.
func sourceTransitionOnly(err error, ownExpiry ...bool) bool {
	if err == nil {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if !sourceTransitionOnly(cause, ownExpiry...) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return sourceTransitionOnly(wrapped.Unwrap(), ownExpiry...)
	}
	return err == context.Canceled || err == inputdesktop.ErrInactive || err == wininput.ErrEmergencyRelease || err == context.DeadlineExceeded && len(ownExpiry) == 1 && ownExpiry[0]
}
