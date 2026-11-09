// Package cleanupresult separates an operation's stop cause from completed
// cleanup. Confirmation is local evidence, never input or native-owner authority.
package cleanupresult

import "reflect"

type confirmed struct{ cause error }

func (e *confirmed) Error() string { return e.cause.Error() }
func (e *confirmed) Unwrap() error { return e.cause }

// Confirm is used only after the cleanup owner has joined its work successfully.
func Confirm(cause error) error {
	if cause == nil {
		return nil
	}
	return &confirmed{cause: cause}
}

// Confirmed accepts a sole confirmed result, including errors.Join(result,nil).
// An additional cleanup error invalidates confirmation even if it wraps a cause.
func Confirmed(err error) bool {
	if _, ok := err.(*confirmed); ok {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		return len(children) == 1 && Confirmed(children[0])
	}
	return false
}

// OnlyCause checks the Fail contract: successful recovery returns the exact
// supplied cause, possibly joined with itself; cleanup errors are extra leaves.
// Do not use errors.Is: a join containing both cause and failure also matches it.
func OnlyCause(err, cause error) bool {
	if err == nil {
		return true
	}
	if cause != nil && reflect.TypeOf(cause).Comparable() && err == cause {
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !OnlyCause(child, cause) {
				return false
			}
		}
		return true
	}
	return false
}
