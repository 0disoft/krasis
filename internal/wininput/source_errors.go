package wininput

// Normalization requires every leaf, not errors.Is matching one sibling of an
// errors.Join. Unknown failure and incomplete cleanup must remain observable.
func sourceErrorsOnly(err error, allowed ...error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		if len(causes) == 0 {
			return false
		}
		for _, cause := range causes {
			if !sourceErrorsOnly(cause, allowed...) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return sourceErrorsOnly(wrapped.Unwrap(), allowed...)
	}
	for _, cause := range allowed {
		if cause != nil && err == cause {
			return true
		}
	}
	return false
}
