package inputdesktop

import "errors"

// Fixed native operation labels, never desktop names, handles or error text.
type operationError struct {
	operation string
	cause     error
}

func (e *operationError) Error() string { return "input desktop operation failed" }
func (e *operationError) Unwrap() error { return e.cause }
func operationFailure(operation string, cause error) error {
	if cause == nil {
		return nil
	}
	return &operationError{operation: operation, cause: cause}
}

func FailureOperation(err error) string {
	operation, _ := FailureDetails(err)
	return operation
}

// The cause belongs to the selected operation, not an unrelated joined error.
// It stays internal to diagnostics; callers must not serialize its raw text.
func FailureDetails(err error) (string, error) {
	var operation *operationError
	if !errors.As(err, &operation) {
		return "", nil
	}
	if inner, cause := FailureDetails(operation.cause); inner != "" {
		return inner, cause
	}
	switch operation.operation {
	case "desktop-bind", "desktop-restore", "desktop-close", "desktop-thread-exit", "desktop-current", "desktop-open", "desktop-name", "desktop-active":
		return operation.operation, operation.cause
	}
	if ValidCaptureThreadFailureOperation(operation.operation) {
		return operation.operation, operation.cause
	}
	return "", nil
}
