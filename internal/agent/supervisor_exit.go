package agent

import "errors"

// Preserve the cause chain and cancellation semantics while naming the branch
// that stopped the supervisor. Never expose the wrapped error text in exports.
type supervisorExitError struct {
	phase string
	cause error
}

func (e *supervisorExitError) Error() string { return e.cause.Error() }
func (e *supervisorExitError) Unwrap() error { return e.cause }

func supervisorExit(phase string, err error) error {
	if err == nil {
		return nil
	}
	return &supervisorExitError{phase: phase, cause: err}
}

func SupervisorExitPhase(err error) string {
	var exit *supervisorExitError
	if !errors.As(err, &exit) {
		return ""
	}
	switch exit.phase {
	case "source-close", "connection-close", "nonlocal-state", "unexpected-stop", "nil-connection":
		return exit.phase
	}
	return ""
}
