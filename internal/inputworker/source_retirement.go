package inputworker

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

// This marker originates only in the authenticated final-retirement reader.
// Ordinary authority/IPC failures cannot masquerade as joined native owners.
var ErrSourceCaptureRetired = errors.New("protected source native owners retired")

func CleanSourceRetirement(err error) bool {
	return errors.Is(err, ErrSourceCaptureRetired) && cleanSourceRetirementOnly(err)
}
func cleanSourceRetirementOnly(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if !cleanSourceRetirementOnly(cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return cleanSourceRetirementOnly(wrapped.Unwrap())
	}
	return err == ErrSourceCaptureRetired || err == wininput.ErrProtectedInput || err == context.Canceled
}

// A final authenticated retirement proves native owners joined. Cancellation
// and EOF may become ready together; select order must not change that proof.
func sourceRetirementResult(ctx context.Context, retired bool, cause error) (error, bool) {
	if !retired || cause != nil {
		return cause, false
	}
	if ctx.Err() != nil {
		return ctx.Err(), true
	}
	return errors.Join(wininput.ErrProtectedInput, ErrSourceCaptureRetired), true
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
