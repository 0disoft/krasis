package inputworker

import (
	"context"
	"errors"
)

// Fixed query admission/outcome codes, separate from native authority denials.
// No error text, handle, process identifier, or peer payload is retained.
type sourceQueryFailure struct {
	owner  string
	reason string
	cause  error
}

func (e *sourceQueryFailure) Error() string { return ErrAuthority.Error() }
func (e *sourceQueryFailure) Unwrap() error { return e.cause }
func (e *sourceQueryFailure) LoginRuntimeOperation() string {
	if e.owner == "service" || e.owner == "agent" {
		return "source-" + e.owner + "-query-" + e.reason
	}
	return "source-query-" + e.reason
}

func sourceQueryFailureOperations() []string {
	var values []string
	for _, prefix := range []string{"source-query-", "source-service-query-", "source-agent-query-"} {
		for _, reason := range []string{"deadline", "cancelled", "capacity", "closed"} {
			values = append(values, prefix+reason)
		}
	}
	return values
}

func queryAdmissionFailure(kind authorityQueryKind, reason string, cause error) error {
	err := ErrAuthority
	if cause != nil {
		err = errors.Join(err, cause)
	}
	if kind != sourceInputAuthorityQuery && kind != sourcePresenceAuthorityQuery && kind != sourceAdvisoryAuthorityQuery && kind != sourceAdvisoryPresenceAuthorityQuery {
		return err
	}
	if cause == context.Canceled {
		reason = "cancelled"
	}
	if cause == context.DeadlineExceeded {
		reason = "deadline"
	}
	return &sourceQueryFailure{reason: reason, cause: err}
}

func sourceQueryOwner(owner string, cause error) error {
	var failure *sourceQueryFailure
	if !errors.As(cause, &failure) {
		return cause
	}
	return &sourceQueryFailure{owner: owner, reason: failure.reason, cause: failure.cause}
}
