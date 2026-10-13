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
	var operation *operationError
	if !errors.As(err, &operation) {
		return ""
	}
	if inner := FailureOperation(operation.cause); inner != "" {
		return inner
	}
	switch operation.operation {
	case "desktop-bind", "desktop-restore", "desktop-close", "desktop-thread-exit":
		return operation.operation
	}
	return ""
}
