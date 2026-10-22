package localapi

// Keep transport/decoder causes available to errors.Is/As without exposing
// endpoint addresses, payloads or credential paths in the display message.
type requestError struct {
	kind  error
	phase string
	cause error
}

func (e *requestError) Error() string { return e.kind.Error() + ": " + e.phase }
func (e *requestError) Unwrap() []error {
	if e.cause == nil {
		return []error{e.kind}
	}
	return []error{e.kind, e.cause}
}
