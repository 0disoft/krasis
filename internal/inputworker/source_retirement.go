package inputworker

import (
	"context"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

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
