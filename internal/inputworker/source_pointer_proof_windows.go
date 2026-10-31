//go:build windows

package inputworker

import (
	"context"
	"errors"
)

// Created only before wire-gate admission. It is not evidence about an attempted
// write, a key/button/wheel transition, or native owner retirement.
type sourcePointerProofExpired struct{ cause error }

func (e *sourcePointerProofExpired) Error() string { return "unsent source pointer proof expired" }
func (e *sourcePointerProofExpired) Unwrap() error { return e.cause }

// Relative pointer samples already coalesce/drop under load. Give their full
// fresh proof and final presence check the bounded optional query owner so a
// late read-only query retains its pins without poisoning unrelated commands.
// A later sample needs new proofs; no query result is cached or replayed.
func (w *sourceWriter) lockPointer(ctx context.Context) error {
	for {
		for _, check := range []func(context.Context) error{
			w.channel.authorizeSourceAdvisory, w.channel.authorizeSourceAdvisoryPresence,
		} {
			if err := check(ctx); err != nil {
				var query *sourceQueryFailure
				if ctx.Err() == context.DeadlineExceeded && errors.As(err, &query) &&
					sourcePointerSubmittedQueryDeadline(query.reason) &&
					errors.Is(query.cause, context.DeadlineExceeded) && sourceQueryRejectionOnly(query.cause) {
					return &sourcePointerProofExpired{cause: err}
				}
				return err
			}
		}
		if w.mu.TryLock() {
			return nil
		}
		if err := w.mu.LockContext(ctx); err != nil {
			return err
		}
		w.mu.Unlock() // Contention invalidates both proofs, just as for discrete input.
	}
}

func sourcePointerSubmittedQueryDeadline(reason string) bool {
	switch reason {
	case "deadline-scheduled", "deadline-checking", "deadline-publishing", "deadline-completed":
		return true
	}
	// A second sample cannot repeatedly wait out the same stuck native owner.
	// Waiting/capacity/admission failures keep their existing terminal policy.
	return false
}
