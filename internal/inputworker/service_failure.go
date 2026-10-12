package inputworker

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
