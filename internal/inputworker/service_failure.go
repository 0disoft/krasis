package inputworker

import "errors"

// Recovery uncertainty still fences the service, but cannot replace the
// authenticated worker's operation/native cause or a concurrent close failure.
func serviceRecoveryFailure(cause, closeErr error) error {
	if errors.Is(cause, ErrRecovery) || closeErr != nil {
		return errors.Join(ErrRecovery, cause, closeErr)
	}
	return nil
}

// Fixed endpoint classification survives service joins and native errors. No
// peer data, process identity, paths or raw error text enter runtime receipts.
type serviceEndpointError struct {
	cause    error
	endpoint string
}

func (e *serviceEndpointError) Error() string              { return "input service endpoint failed" }
func (e *serviceEndpointError) Unwrap() error              { return e.cause }
func (e *serviceEndpointError) LoginRuntimeReason() string { return e.endpoint }
func serviceEndpointFailure(err error, endpoint string) error {
	if err == nil {
		return nil
	}
	return &serviceEndpointError{cause: err, endpoint: endpoint}
}
