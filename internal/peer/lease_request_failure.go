package peer

// Fixed local phase only. No identifiers, payload, desktop or native text are
// part of this error's message; the original cause remains inspectable.
type LeaseRequestFailure struct {
	Stage string
	Cause error
}

func (e *LeaseRequestFailure) Error() string { return "lease request preparation failed" }
func (e *LeaseRequestFailure) Unwrap() error { return e.Cause }
