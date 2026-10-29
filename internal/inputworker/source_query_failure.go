package inputworker

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/logindiag"
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

func validSourceQueryOperation(operation string) bool {
	if logindiag.ValidSourceQueryDeadlineOperation(operation) {
		return true
	}
	for _, candidate := range sourceQueryFailureOperations() {
		if candidate == operation {
			return true
		}
	}
	return false
}

// This reader rejected a command before dispatching any native input work.
// Its failure is not proof that query handles retired: the final-report caller
// must independently fence pending queries before sending retirement status.
func sourceReadQueryRejected(err error) bool {
	reader, ok := err.(*sourceReaderFailure)
	if !ok || reader.operation != "source-reader-authority" {
		return false
	}
	query, ok := reader.cause.(*sourceQueryFailure)
	return ok && validSourceQueryOperation(query.LoginRuntimeOperation()) && errors.Is(query.cause, ErrAuthority) && sourceQueryRejectionOnly(query.cause)
}

func sourceQueryRejectionOnly(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if !sourceQueryRejectionOnly(cause) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return sourceQueryRejectionOnly(wrapped.Unwrap())
	}
	return err == ErrAuthority || err == context.Canceled || err == context.DeadlineExceeded
}

func queryAdmissionFailure(kind authorityQueryKind, reason string, cause error) error {
	err := ErrAuthority
	if cause != nil {
		err = errors.Join(err, cause)
	}
	if kind != sourceInputAuthorityQuery && kind != sourcePresenceAuthorityQuery && kind != sourceAdvisoryAuthorityQuery && kind != sourceAdvisoryPresenceAuthorityQuery && kind != sourceWatchdogAuthorityQuery {
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
